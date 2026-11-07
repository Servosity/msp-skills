// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package mcp

import (
	"net/http"

	"microsoft-graph-pp-cli/internal/client"
)

// No in-process test runs credkit.Start, so the outbound guard would refuse
// every credentialed request to a loopback test server (client.OutboundGuard).
// The built binaries' tests in internal/msgraphblackbox prove the guard, and
// TestOutboundGuardIsTheKit (internal/client) that the programs install it.
func init() {
	client.OutboundGuard = func(rt http.RoundTripper) http.RoundTripper { return rt }
}
