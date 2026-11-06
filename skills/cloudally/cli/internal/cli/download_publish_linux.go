// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//go:build linux

package cli

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

// renameNoReplace moves oldBase to newBase inside dir and fails if newBase
// exists; RENAME_NOREPLACE makes the check and the move one step. It
// answers errors.ErrUnsupported when the drive cannot do that.
func renameNoReplace(_ *os.Root, dir *os.File, _, oldBase, newBase string) error {
	fd := int(dir.Fd())
	err := unix.Renameat2(fd, oldBase, fd, newBase, unix.RENAME_NOREPLACE)
	// A filesystem or kernel without the flag answers EINVAL or ENOSYS.
	if errors.Is(err, errors.ErrUnsupported) || err == unix.EINVAL {
		return errors.ErrUnsupported
	}
	return err
}
