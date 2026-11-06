// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated; handfixes.json credkit-wiring): the two seams
// through which the connector reaches the shared credential kit.

package cliutil

import "knowbe4-pp-cli/internal/credkit"

// OutboundGuard is the credential kit's outbound guard, around every request
// the Reporting and User Event clients send. Production code never reassigns
// it. In-process unit tests do, to the identity: they never run credkit.Start,
// so the kit has no declarations and no base origin and would refuse every
// credentialed request to a loopback test server. The built binaries' tests
// prove the guard itself.
var OutboundGuard = credkit.Transport

// HoldCredential is credkit.Hold, through which the connector holds every
// credential it reads after startup (a token auth set-token reads from stdin,
// a minted Authorization header). Production code never reassigns it.
// In-process unit tests do, to a no-op: a hold lasts until the process exits.
// Tests of holding run in a child process (runInChild in their _test files).
var HoldCredential = credkit.Hold
