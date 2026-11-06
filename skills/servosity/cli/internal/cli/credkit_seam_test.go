// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"net/http"

	"servosity-msp-pp-cli/internal/client"
	"servosity-msp-pp-cli/internal/cliutil"
)

// This package's unit tests run without the credential kit's outbound guard
// (client.OutboundGuard): no test here runs credkit.Start, so the guard would
// refuse every credentialed request to the loopback servers these tests stand
// up. Nor do they hold config values (cliutil.HoldConfigFile), which one test
// would carry into the next. Both are proven on the built binaries by
// internal/credwire.
func init() {
	client.OutboundGuard = func(rt http.RoundTripper) http.RoundTripper { return rt }
	cliutil.HoldConfigFile = func([]byte) {}
}
