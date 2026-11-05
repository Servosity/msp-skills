// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credkit

import "syscall"

const fionread = syscall.TIOCINQ // FIONREAD

func pipe(p []int) error { return syscall.Pipe2(p, syscall.O_CLOEXEC) }

func dup2(from, to int) error { return syscall.Dup3(from, to, 0) }
