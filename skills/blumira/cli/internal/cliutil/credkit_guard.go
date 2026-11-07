// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated; handfixes.json credkit-wiring).

package cliutil

import "blumira-pp-cli/internal/credkit"

// OutboundGuard is the credential kit's outbound guard, around every transport
// the API client (internal/client), the auth login token mint, the --deliver
// webhook and the Spacefast client send through. The probe and feedback
// clients call credkit.Transport directly.
// Production code never reassigns it. In-process unit tests do, to the
// identity: they never run credkit.Start, so the kit has no declarations and no
// base origin and would refuse every credentialed request to a loopback test
// server. The built binaries' leak tests (internal/credwire) prove the guard
// itself.
var OutboundGuard = credkit.Transport
