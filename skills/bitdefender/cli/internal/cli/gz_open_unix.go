// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

//go:build !windows

package cli

import (
	"os"
	"syscall"
)

// openNoBlock opens path read-only without blocking on a FIFO that has no
// writer; callers then reject anything that is not a regular file.
func openNoBlock(path string) (*os.File, error) {
	// #nosec G304 -- path is the operator-named --devices/--agreements export; readBoundedFile
	// Lstat-checks it is a regular file, re-verifies the opened descriptor and caps the read at 50 MiB.
	return os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
}
