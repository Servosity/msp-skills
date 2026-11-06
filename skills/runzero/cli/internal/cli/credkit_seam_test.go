// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"net/http"

	"runzero-pp-cli/internal/client"
	"runzero-pp-cli/internal/cliutil"
	"runzero-pp-cli/internal/credkit"
)

// init runs this package's unit tests without the credential kit's holds and
// outbound guard (handfixes.json credkit-wiring), before the generated
// TestMain: no test here runs credkit.Start, so a value one test holds would
// withhold another test's writes, and the guard would refuse every
// credentialed request to the loopback servers these tests stand up. The built
// binaries' black-box tests (internal/runzeroblackbox) prove both.
func init() {
	client.OutboundGuard = func(rt http.RoundTripper) http.RoundTripper { return rt }
	cliutil.Hold = func(credkit.Source, ...string) {}
	cliutil.HoldBody = func([]byte) {}
	cliutil.HoldURL = func(credkit.Source, string) {}
}
