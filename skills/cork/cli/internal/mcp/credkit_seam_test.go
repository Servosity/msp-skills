// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (handfixes.json credkit-wiring).

package mcp

import (
	"net/http"

	"cork-pp-cli/internal/cliutil"
	"cork-pp-cli/internal/credkit"
)

// No test here runs credkit.Start, so the kit has no declarations and would
// refuse every credentialed request to a loopback test server, and a value one
// test holds would withhold the next test's output. The built binaries' leak
// tests (internal/credwire) prove the kit itself.
func init() {
	cliutil.OutboundGuard = func(rt http.RoundTripper) http.RoundTripper {
		if rt == nil {
			return lazyKitSeamTransport{}
		}
		return rt
	}
	cliutil.Hold = func(credkit.Source, ...string) {}
	cliutil.HoldBody = func([]byte) {}
}

// lazyKitSeamTransport sends with whatever http.DefaultTransport is when the
// request goes out, as a client with no Transport does; tests swap it.
type lazyKitSeamTransport struct{}

func (lazyKitSeamTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	return http.DefaultTransport.RoundTrip(r)
}
