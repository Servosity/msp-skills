//go:build windows

// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"os"
	"syscall"
)

// interruptSignals cancel the command's context (root.go Execute). Windows
// has no SIGHUP; closing a console window delivers the interrupt.
var interruptSignals = []os.Signal{os.Interrupt, syscall.SIGTERM}
