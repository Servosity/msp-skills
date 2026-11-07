// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated; handfixes.json credkit-wiring).

package cliutil

import "datto-rmm-pp-cli/internal/credkit"

// Hold and HoldBody are the credential kit's holders for a value this program
// acquires after credkit.Start. Variables so the unit tests that run no
// credkit.Start can keep one test's credential from being held for the rest of
// the process, where the kit would withhold every later test's file carrying it.
var (
	Hold     = credkit.Hold
	HoldBody = credkit.HoldBody
)
