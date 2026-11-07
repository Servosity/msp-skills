// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cliutil

import "net/http"

// In-process tests run without the credential kit's outbound guard: none runs
// credkit.Start, so the guard would refuse a credentialed request to a
// loopback test server. The built binaries' tests prove the guard itself.
func init() {
	OutboundGuard = func(rt http.RoundTripper) http.RoundTripper { return rt }
}
