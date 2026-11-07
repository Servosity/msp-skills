// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credkit

import "syscall"

const fionread = 0x4004667f // FIONREAD: _IOR('f', 127, int)

// pipe runs under syscall.ForkLock (redirectStd), so no child is started
// between creating the pipe and marking it close-on-exec.
func pipe(p []int) error {
	if err := syscall.Pipe(p); err != nil {
		return err
	}
	syscall.CloseOnExec(p[0])
	syscall.CloseOnExec(p[1])
	return nil
}

func dup2(from, to int) error { return syscall.Dup2(from, to) }
