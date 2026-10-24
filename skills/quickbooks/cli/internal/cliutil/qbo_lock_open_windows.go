//go:build windows

// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (issue #492). Not generated. Recorded hand-fix
// `qbo-token-refresh` in handfixes.json.

package cliutil

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

// qboOpenLockFile opens (creating) the token refresh's lock file. It opens
// the path itself, never what a symlink or junction there points to
// (FILE_FLAG_OPEN_REPARSE_POINT), and refuses a reparse point or anything
// but a regular file.
func qboOpenLockFile(path string) (*os.File, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	h, err := windows.CreateFile(p,
		windows.GENERIC_READ|windows.GENERIC_WRITE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil, windows.OPEN_ALWAYS,
		windows.FILE_ATTRIBUTE_NORMAL|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return nil, err
	}
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(h, &info); err != nil {
		_ = windows.CloseHandle(h)
		return nil, err
	}
	if info.FileAttributes&(windows.FILE_ATTRIBUTE_REPARSE_POINT|windows.FILE_ATTRIBUTE_DIRECTORY) != 0 {
		_ = windows.CloseHandle(h)
		return nil, errors.New("not a regular file")
	}
	return os.NewFile(uintptr(h), path), nil
}

// qboDirSyncUnsupported is unused on Windows, where qboSyncDir does not open
// directories.
func qboDirSyncUnsupported(error) bool { return false }
