// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

//go:build windows

package cli

import "golang.org/x/sys/windows"

// esetIsTerminalFd reports whether fd is a console: only a console handle
// answers GetConsoleMode. NUL is a character device but not a console
// (handfixes.json: terminal-check-is-a-real-tty).
const esetHasTerminalQuery = true

func esetIsTerminalFd(fd uintptr) bool {
	var mode uint32
	return windows.GetConsoleMode(windows.Handle(fd), &mode) == nil
}
