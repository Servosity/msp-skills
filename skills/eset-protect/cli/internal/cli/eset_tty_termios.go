// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

//go:build aix || linux || solaris

package cli

import "golang.org/x/sys/unix"

// esetTermiosRequest reads the terminal attributes (TCGETS).
const esetTermiosRequest = unix.TCGETS
