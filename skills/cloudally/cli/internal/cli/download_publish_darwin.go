// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//go:build darwin

package cli

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

// renameNoReplace moves oldBase to newBase inside dir and fails if newBase
// exists; RENAME_EXCL makes the check and the move one step. It answers
// errors.ErrUnsupported when the drive cannot do that.
func renameNoReplace(_ *os.Root, dir *os.File, _, oldBase, newBase string) error {
	fd := int(dir.Fd())
	err := unix.RenameatxNp(fd, oldBase, fd, newBase, unix.RENAME_EXCL)
	// exFAT and FAT volumes refuse the flag.
	if errors.Is(err, errors.ErrUnsupported) || err == unix.EINVAL {
		return errors.ErrUnsupported
	}
	return err
}
