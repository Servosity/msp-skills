// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package cli

import "golang.org/x/sys/unix"

// esetIsTerminalFd reports whether fd is a real terminal: only a tty answers
// the termios request. /dev/null is a character device but not a terminal
// (handfixes.json: terminal-check-is-a-real-tty).
const esetHasTerminalQuery = true

func esetIsTerminalFd(fd uintptr) bool {
	_, err := unix.IoctlGetTermios(int(fd), esetTermiosRequest)
	return err == nil
}
