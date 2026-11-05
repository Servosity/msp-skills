// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package platform

import (
	"net/http"

	"synology-pp-cli/internal/cliutil"
	"synology-pp-cli/internal/credkit"
)

// No in-process test runs credkit.Start, so the kit's outbound guard would refuse
// every credentialed request to a loopback test server, and a value held by one
// test would withhold the next test's output. The kit is proven on the built
// binaries by the leak tests in internal/credwire.
func init() {
	cliutil.OutboundGuard = func(rt http.RoundTripper) http.RoundTripper { return rt }
	cliutil.Hold = func(credkit.Source, ...string) {}
}
