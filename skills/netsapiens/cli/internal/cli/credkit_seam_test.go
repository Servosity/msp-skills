// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"net/http"

	"netsapiens-pp-cli/internal/client"
	"netsapiens-pp-cli/internal/cliutil"
)

// No test in this package runs credkit.Start, so the outbound guard would refuse
// every credentialed request to a loopback server. The kit itself is proven on the
// built binaries by the leak tests in internal/credwire.
func init() {
	client.OutboundGuard = func(rt http.RoundTripper) http.RoundTripper { return rt }
	cliutil.Hold = func(string, ...string) {}
}
