//go:build !windows

// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"os"
	"syscall"
)

// interruptSignals cancel the command's context (root.go Execute), so a
// restore or download run stops at a safe point and prints how to resume.
// SIGHUP is here because closing a terminal or dropping an SSH session
// sends it; its default action would kill a run in the middle of creating
// jobs and leave claims marked in flight.
var interruptSignals = []os.Signal{os.Interrupt, syscall.SIGTERM, syscall.SIGHUP}
