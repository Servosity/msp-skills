// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cliutil

import "appdirect-pp-cli/internal/credkit"

// OutboundGuard is the credential kit's outbound guard, around every request
// the client, the probe, the token exchange, feedback and --deliver send
// (handfixes.json credkit-wiring). Production code never reassigns it.
// In-process unit tests do, to the identity: they never run credkit.Start, so
// the kit has no declarations and no base origin and would refuse every
// credentialed request to a loopback test server. The built binaries' tests
// (internal/credwire kit_classes_test.go) prove the guard itself.
var OutboundGuard = credkit.Transport
