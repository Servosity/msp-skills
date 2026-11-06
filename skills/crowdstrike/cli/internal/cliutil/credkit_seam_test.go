// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cliutil

import "net/http"

// In-process tests never run credkit.Start, so the kit has no declarations: its
// guard would refuse every credentialed request to a loopback test server, and
// a value one test holds would withhold another test's unrelated output in the
// same process. The built binaries' tests (internal/crowdstrikeblackbox) prove
// the guard and the holds.
func init() {
	OutboundGuard = func(rt http.RoundTripper) http.RoundTripper { return rt }
	HoldValues = func(string, ...string) {}
	HoldBody = func([]byte) {}
	HoldURL = func(string, string) {}
}
