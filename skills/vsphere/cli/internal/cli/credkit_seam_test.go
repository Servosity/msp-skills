// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"net/http"

	"vsphere-pp-cli/internal/client"
)

// No in-process test runs credkit.Start, so the outbound guard would refuse
// every credentialed request to a loopback test server (client.OutboundGuard).
// The built binaries' tests in internal/vsphereblackbox prove the guard.
func init() {
	client.OutboundGuard = func(rt http.RoundTripper) http.RoundTripper { return rt }
}
