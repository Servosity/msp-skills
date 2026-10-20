// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

//go:build !(aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris || windows)

package cli

// esetHasTerminalQuery is false here: there is no terminal query on this
// platform, so esetTerminalFile falls back to the character-device test.
const esetHasTerminalQuery = false

func esetIsTerminalFd(fd uintptr) bool { return false }
