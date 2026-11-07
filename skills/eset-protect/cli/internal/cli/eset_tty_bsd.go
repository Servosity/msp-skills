// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

//go:build darwin || dragonfly || freebsd || netbsd || openbsd

package cli

import "golang.org/x/sys/unix"

// esetTermiosRequest reads the terminal attributes (TIOCGETA on the BSDs).
const esetTermiosRequest = unix.TIOCGETA
