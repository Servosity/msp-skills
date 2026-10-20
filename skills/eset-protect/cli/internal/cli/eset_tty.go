// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored (not generator output); pinned by skills/eset-protect/handfixes.json
// under "terminal-check-is-a-real-tty".
//
// The generated isTerminal (helpers.go) treated every character device as a
// terminal, so '> /dev/null' on macOS and Linux and '> NUL' on Windows counted
// as a terminal: the quarantine downloads refused them with exit 2, and
// output to them was formatted as a human table. A real terminal is one that
// answers the termios request (unix) or GetConsoleMode (Windows); only a
// platform with neither keeps the character-device test.
package cli

import "os"

// esetTerminalFile is the one terminal test isTerminal uses for an *os.File.
// It is a variable only so tests can stand a terminal in for a pipe.
var esetTerminalFile = func(f *os.File) bool {
	if !esetHasTerminalQuery {
		fi, err := f.Stat()
		if err != nil {
			return true
		}
		return fi.Mode()&os.ModeCharDevice != 0
	}
	rc, err := f.SyscallConn()
	if err != nil {
		return false
	}
	is := false
	if cerr := rc.Control(func(fd uintptr) { is = esetIsTerminalFd(fd) }); cerr != nil {
		return false
	}
	return is
}
