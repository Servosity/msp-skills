// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

//go:build !windows

package fleet

import (
	"os"
	"syscall"
)

func lockShared(f *os.File) error { return syscall.Flock(int(f.Fd()), syscall.LOCK_SH|syscall.LOCK_NB) }

func lockExclusive(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
}
