// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package config

import (
	"net/http"

	"crowdstrike-pp-cli/internal/cliutil"
)

// In-process tests never run credkit.Start, so the kit has no declarations: its
// guard would refuse every credentialed request to a loopback test server, and
// a value one test holds would withhold another test's unrelated output in the
// same process. The built binaries' tests (internal/crowdstrikeblackbox) prove
// the guard and the holds.
func init() {
	cliutil.OutboundGuard = func(rt http.RoundTripper) http.RoundTripper { return rt }
	cliutil.HoldValues = func(string, ...string) {}
	cliutil.HoldBody = func([]byte) {}
	cliutil.HoldURL = func(string, string) {}
}
