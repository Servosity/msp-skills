// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"net/http"

	"apple-business-pp-cli/internal/cliutil"
)

// No test here runs credkit.Start, so the kit's outbound guard has no declarations
// and would refuse every credentialed request to a loopback test server. The built
// binaries' tests (internal/applebusinessblackbox) prove the guard itself.
func init() {
	cliutil.OutboundGuard = func(rt http.RoundTripper) http.RoundTripper { return rt }
}
