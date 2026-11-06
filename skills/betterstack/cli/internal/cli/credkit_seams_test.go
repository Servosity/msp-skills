// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// Hand-written (handfixes.json credkit-wiring): no test in this package runs
// credkit.Start, so the kit has no declarations and no base origin and would
// refuse every credentialed request to a loopback test server, and a value one
// test holds would withhold the next test's files. The built binaries' leak
// tests (internal/credwire) prove the kit itself.

import (
	"net/http"

	"betterstack-pp-cli/internal/client"
	"betterstack-pp-cli/internal/cliutil"
	"betterstack-pp-cli/internal/credkit"
)

func init() {
	client.OutboundGuard = func(rt http.RoundTripper) http.RoundTripper { return rt }
	cliutil.Hold = func(credkit.Source, ...string) {}
	cliutil.HoldBody = func([]byte) {}
	cliutil.HoldURL = func(credkit.Source, string) {}
}
