// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cliutil

// Hand-written (handfixes.json credkit-wiring).

import "runzero-pp-cli/internal/credkit"

// Hold, HoldBody and HoldURL are the credential kit's holders, which every hold
// this program makes after startup goes through. Production code never
// reassigns them. In-process unit tests do, to no-ops: they never run
// credkit.Start, and a value one test holds would withhold another test's
// writes for the rest of the process. The built binaries' black-box tests
// (internal/runzeroblackbox) prove the holds themselves.
var (
	Hold     = credkit.Hold
	HoldBody = credkit.HoldBody
	HoldURL  = credkit.HoldURL
)
