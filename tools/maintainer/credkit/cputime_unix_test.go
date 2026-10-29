// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

//go:build unix

package credkit

import "syscall"

// cpuTime is the process's user and system CPU time in milliseconds: on a
// loaded machine it moves far less than the wall clock.
func cpuTime() float64 {
	var ru syscall.Rusage
	if syscall.Getrusage(syscall.RUSAGE_SELF, &ru) != nil {
		return 0
	}
	return float64(ru.Utime.Nano()+ru.Stime.Nano()) / 1e6
}
