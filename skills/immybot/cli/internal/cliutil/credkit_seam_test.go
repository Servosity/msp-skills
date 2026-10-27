// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cliutil

import (
	"net/http"
)

// In-process tests never run credkit.Start, so the kit has no declarations and
// would refuse every credentialed request to a loopback test server. The built
// binaries' tests (cmd/immybot-mcp) prove the guard itself.
func init() {
	OutboundGuard = func(rt http.RoundTripper) http.RoundTripper { return rt }
	HoldValues = func(string, ...string) {}
}
