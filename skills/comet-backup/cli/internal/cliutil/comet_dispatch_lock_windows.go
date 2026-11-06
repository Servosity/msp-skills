//go:build windows

// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cliutil

// comet-backup handfix (rerun-dispatch-lock): the Windows dispatch lock is a
// named kernel mutex, Global\comet-backup-cli-<name>-<account SID>.
//
// Why a kernel object and not a file: every per-account directory on Windows
// is found through the environment (USERPROFILE, LOCALAPPDATA, APPDATA,
// TEMP/TMP, os.UserHomeDir, os.TempDir) or through profile and known-folder
// lookups that store REG_EXPAND_SZ paths (%USERPROFILE%, %SystemDrive%) which
// can expand against the process environment, so a file lock would again
// follow what the invocation says. The SID comes from the process token, and
// the object namespace reads no environment at all.
//
// Why Global\ and not Local\: Local\ is per logon session, so a scheduled task
// (session 0), a service and an interactive or RDP run of the same account
// would each get their own mutex and could all dispatch. Global\ is the one
// namespace every session shares; creating a mutex there needs no privilege
// (only file mappings need SeCreateGlobalPrivilege).
//
// The OS releases ownership when a holder dies: the next wait returns
// WAIT_ABANDONED and takes it, the guarantee the Unix flock gives. Ownership
// belongs to a thread, so the goroutine stays locked to its OS thread from the
// wait to the release.
//
// Squatting: Global\ is shared by every account, so another one could create
// the name first. The object is opened with only the rights the lock needs
// (SYNCHRONIZE, MUTEX_MODIFY_STATE, READ_CONTROL), and its owner must be this
// account, BUILTIN\Administrators or LocalSystem; a foreign owner, an object
// this process may not open, or another object type under the name all fail
// closed. An elevated run of this account may create the object with a
// default DACL its non-elevated token cannot open; a non-elevated run that
// overlaps it is then refused rather than let through.

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"golang.org/x/sys/windows"
)

const (
	dispatchLockMutexAccess = windows.SYNCHRONIZE | windows.MUTEX_MODIFY_STATE | windows.READ_CONTROL
	dispatchLockWaitTimeout = 0x00000102 // WAIT_TIMEOUT
)

func dispatchLockObjectName(name, sid string) string {
	return `Global\comet-backup-cli-` + name + `-` + sid
}

// dispatchLockIdentity is the kernel object name of the lock called name.
func dispatchLockIdentity(name string) (string, error) {
	sid, err := currentUserSID()
	if err != nil {
		return "", err
	}
	return dispatchLockObjectName(name, sid), nil
}

func tryOSDispatchLock(name string, fn func() error) (bool, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	tok, err := windows.OpenCurrentProcessToken()
	if err != nil {
		return false, fmt.Errorf("reading this account's identity for the dispatch lock: %w", err)
	}
	defer tok.Close()
	user, err := tok.GetTokenUser()
	if err != nil {
		return false, fmt.Errorf("reading this account's identity for the dispatch lock: %w", err)
	}
	me := user.User.Sid
	objName := dispatchLockObjectName(name, me.String())
	p, err := windows.UTF16PtrFromString(objName)
	if err != nil {
		return false, fmt.Errorf("dispatch lock %s: %w", objName, err)
	}
	h, err := windows.CreateMutexEx(nil, p, 0, dispatchLockMutexAccess)
	if h == 0 {
		switch {
		case errors.Is(err, windows.ERROR_ACCESS_DENIED):
			return false, dispatchLockRefusal("the lock object %s exists but this process may not open it (an elevated run of this account, or another account, holds it); run again after it ends", objName)
		case errors.Is(err, windows.ERROR_INVALID_HANDLE):
			return false, dispatchLockRefusal("another kind of object already uses the name %s", objName)
		}
		return false, fmt.Errorf("creating the dispatch lock %s: %w", objName, err)
	}
	defer func() { _ = windows.CloseHandle(h) }()
	if err := checkDispatchLockOwner(h, me, objName); err != nil {
		return false, err
	}
	ev, err := windows.WaitForSingleObject(h, 0)
	if err != nil {
		return false, fmt.Errorf("taking the dispatch lock %s: %w", objName, err)
	}
	switch ev {
	case windows.WAIT_OBJECT_0, windows.WAIT_ABANDONED:
	case dispatchLockWaitTimeout:
		return false, nil
	default:
		return false, fmt.Errorf("taking the dispatch lock %s: unexpected wait result %#x", objName, ev)
	}
	defer func() { _ = windows.ReleaseMutex(h) }()
	return true, fn()
}

// checkDispatchLockOwner refuses a mutex another account created.
func checkDispatchLockOwner(h windows.Handle, me *windows.SID, objName string) error {
	sd, err := windows.GetSecurityInfo(h, windows.SE_KERNEL_OBJECT, windows.OWNER_SECURITY_INFORMATION)
	if err != nil {
		return dispatchLockRefusal("cannot read the owner of %s: %v", objName, err)
	}
	owner, _, err := sd.Owner()
	if err != nil || owner == nil {
		return dispatchLockRefusal("cannot read the owner of %s", objName)
	}
	if owner.Equals(me) || owner.IsWellKnown(windows.WinBuiltinAdministratorsSid) || owner.IsWellKnown(windows.WinLocalSystemSid) {
		return nil
	}
	return dispatchLockRefusal("%s is owned by another account (%s)", objName, owner.String())
}

// dispatchIntentDir is under this account's profile, found from its token and not
// from USERPROFILE or LOCALAPPDATA, which differ between two invocations.
func dispatchIntentDir() (string, error) {
	token, err := windows.OpenCurrentProcessToken()
	if err != nil {
		return "", fmt.Errorf("finding this account's profile: %w", err)
	}
	defer token.Close()
	profile, err := token.GetUserProfileDirectory()
	if err != nil {
		return "", fmt.Errorf("finding this account's profile: %w", err)
	}
	dir := filepath.Join(profile, "AppData", "Local", "comet-backup-locks")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("creating the dispatch record directory: %w", err)
	}
	return dir, nil
}
