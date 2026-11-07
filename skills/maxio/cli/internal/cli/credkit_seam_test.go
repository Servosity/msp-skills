// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"net/http"

	"maxio-pp-cli/internal/client"
)

// This package's unit tests run without the credential kit's outbound guard
// (see client.OutboundGuard): no test here runs credkit.Start, so the guard
// would refuse every credentialed request to the loopback fakes these tests
// stand up. The guard is proven on the built binaries by the leak tests in
// internal/maxioblackbox.
func init() {
	client.OutboundGuard = func(rt http.RoundTripper) http.RoundTripper { return rt }
}
