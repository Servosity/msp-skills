//go:build !windows

// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (issue #492). Not generated. Recorded hand-fix
// `qbo-token-refresh` in handfixes.json.

package cliutil

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
)

// qboOpenLockFile opens (creating, mode 0600) the token refresh's lock file
// and refuses a symlink planted at its path (O_NOFOLLOW, so ELOOP) instead
// of opening, and possibly writing to, its target; anything but a regular
// file is refused too.
func qboOpenLockFile(path string) (*os.File, error) {
	f, err := os.OpenFile(filepath.Clean(path), os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW, 0o600)
	if err != nil {
		return nil, err
	}
	st, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	if !st.Mode().IsRegular() {
		_ = f.Close()
		return nil, errors.New("not a regular file")
	}
	return f, nil
}

// qboDirSyncUnsupported reports a directory fsync the file system does not
// support, which is not a failure to make the rename durable.
func qboDirSyncUnsupported(err error) bool {
	return errors.Is(err, syscall.EINVAL) || errors.Is(err, syscall.ENOTSUP) || errors.Is(err, syscall.EOPNOTSUPP)
}
