// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"net/http"

	"connectwise-control-pp-cli/internal/client"
)

// No in-process test runs credkit.Start, so the outbound guard would refuse
// every credentialed request to a loopback test server and then fail closed
// for the whole test process. The built binaries' tests (internal/credwire,
// internal/cwcblackbox) prove the guard itself.
func init() {
	client.OutboundGuard = func(rt http.RoundTripper) http.RoundTripper { return rt }
}
