//go:build !windows

// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cliutil

// comet-backup handfix (rerun-dispatch-lock): the Unix dispatch lock. See
// comet_dispatch_lock.go.

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

// dispatchLockParent is a fixed absolute path, never os.TempDir (TMPDIR) or
// any other lookup an invocation can redirect. Its sticky bit keeps other
// accounts from renaming or removing the per-account directory inside it.
const dispatchLockParent = "/tmp"

// dispatchLockDir is this account's lock directory, named by its effective
// uid: the identity the kernel checks ownership against.
func dispatchLockDir() string {
	return filepath.Join(dispatchLockParent, "comet-backup-locks-"+strconv.Itoa(os.Geteuid()))
}

// dispatchLockIdentity is where the lock called name lives.
func dispatchLockIdentity(name string) (string, error) {
	return filepath.Join(dispatchLockDir(), name+".lock"), nil
}

func tryOSDispatchLock(name string, fn func() error) (bool, error) {
	return tryDispatchLockIn(dispatchLockDir(), name, fn)
}

// tryDispatchLockIn takes the lock name in dir, which must prove private to
// this account first (openPrivateLockDir). The lock file is opened relative to
// that verified directory, so nothing can be swapped in between.
func tryDispatchLockIn(dir, name string, fn func() error) (bool, error) {
	dir = filepath.Clean(dir)
	d, err := openPrivateLockDir(dir)
	if err != nil {
		return false, err
	}
	defer d.Close()
	f, err := openDispatchLockFile(d, dir, name+".lock")
	if err != nil {
		return false, err
	}
	defer f.Close()
	if err := tryLockFile(f); err != nil {
		if errors.Is(err, errFileLockBusy) {
			return false, nil
		}
		return false, fmt.Errorf("taking the dispatch lock %s: %w", f.Name(), err)
	}
	defer func() { _ = unlockFile(f) }()
	// A fresh mtime keeps /tmp cleaners (macOS periodic, systemd-tmpfiles),
	// which skip recently modified files, from unlinking a held lock and so
	// letting a later run lock a new file beside it.
	now := time.Now()
	_ = os.Chtimes(f.Name(), now, now)
	return true, fn()
}

// openPrivateLockDir creates dir 0700 when it is missing and opens it, then
// refuses it unless the opened directory is the one checked, a real directory,
// owned by this account, not group or world writable, with no darwin ACL entry
// granting another principal write.
func openPrivateLockDir(dir string) (*os.File, error) {
	if err := os.Mkdir(dir, 0o700); err != nil && !errors.Is(err, fs.ErrExist) {
		return nil, fmt.Errorf("creating the dispatch lock directory %s: %w", dir, err)
	}
	checked, err := os.Lstat(dir)
	if err != nil {
		return nil, fmt.Errorf("checking the dispatch lock directory %s: %w", dir, err)
	}
	if checked.Mode()&fs.ModeSymlink != 0 {
		return nil, dispatchLockRefusal("%s is a symlink, not a directory this account owns; remove it and run again", dir)
	}
	if !checked.IsDir() {
		return nil, dispatchLockRefusal("%s is not a directory; remove it and run again", dir)
	}
	d, err := os.OpenFile(filepath.Clean(dir), os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_DIRECTORY, 0)
	if err != nil {
		if isSymlinkOpenErr(err) || errors.Is(err, syscall.ENOTDIR) {
			return nil, dispatchLockRefusal("%s was replaced while it was checked", dir)
		}
		return nil, fmt.Errorf("opening the dispatch lock directory %s: %w", dir, err)
	}
	if err := verifyPrivateLockDir(d, checked, dir); err != nil {
		_ = d.Close()
		return nil, err
	}
	return d, nil
}

func verifyPrivateLockDir(d *os.File, checked fs.FileInfo, dir string) error {
	fi, err := d.Stat()
	if err != nil {
		return fmt.Errorf("checking the dispatch lock directory %s: %w", dir, err)
	}
	if !fi.IsDir() || !os.SameFile(checked, fi) {
		return dispatchLockRefusal("%s was replaced while it was checked", dir)
	}
	if uid, ok := unixFileUID(fi); !ok || int64(uid) != int64(os.Geteuid()) {
		return dispatchLockRefusal("%s is owned by another account; remove it (an administrator may need to) and run again", dir)
	}
	if perm := fi.Mode().Perm(); perm&0o022 != 0 {
		return dispatchLockRefusal("%s is writable by other accounts (mode %#o); run: chmod 700 %s", dir, perm, dir)
	}
	if err := rejectDarwinForeignFile(d, dir, "dispatch lock directory", darwinACLScopeDir); err != nil {
		return dispatchLockRefusal("%v", err)
	}
	return nil
}

// openDispatchLockFile opens (creating 0600) name inside the verified
// directory d, refusing a symlink or anything but a regular file this account
// owns.
func openDispatchLockFile(d *os.File, dir, name string) (*os.File, error) {
	path := filepath.Join(dir, name)
	fd, err := unix.Openat(int(d.Fd()), name, unix.O_RDWR|unix.O_CREAT|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0o600)
	if err != nil {
		if isSymlinkOpenErr(err) || errors.Is(err, syscall.EISDIR) {
			return nil, dispatchLockRefusal("%s is not a regular file; remove it and run again", path)
		}
		return nil, fmt.Errorf("opening the dispatch lock %s: %w", path, err)
	}
	f := os.NewFile(uintptr(fd), path)
	fi, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("checking the dispatch lock %s: %w", path, err)
	}
	if !fi.Mode().IsRegular() {
		_ = f.Close()
		return nil, dispatchLockRefusal("%s is not a regular file; remove it and run again", path)
	}
	if uid, ok := unixFileUID(fi); !ok || int64(uid) != int64(os.Geteuid()) {
		_ = f.Close()
		return nil, dispatchLockRefusal("%s is owned by another account; remove it and run again", path)
	}
	return f, nil
}

// dispatchIntentDir is the lock directory, proven private first.
func dispatchIntentDir() (string, error) {
	dir := filepath.Clean(dispatchLockDir())
	d, err := openPrivateLockDir(dir)
	if err != nil {
		return "", err
	}
	_ = d.Close()
	return dir, nil
}
