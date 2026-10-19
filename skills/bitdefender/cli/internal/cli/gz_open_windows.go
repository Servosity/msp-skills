// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

//go:build windows

package cli

import "os"

// openNoBlock opens path read-only; Windows has no FIFO open that blocks here.
func openNoBlock(path string) (*os.File, error) {
	// #nosec G304 -- path is the operator-named --devices/--agreements export; readBoundedFile
	// Lstat-checks it is a regular file, re-verifies the opened descriptor and caps the read at 50 MiB.
	return os.Open(path)
}
