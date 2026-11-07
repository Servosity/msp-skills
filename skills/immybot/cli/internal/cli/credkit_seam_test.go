// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"net/http"

	"immybot-pp-cli/internal/cliutil"
)

// In-process tests never run credkit.Start, so the kit has no declarations and
// would refuse every credentialed request to a loopback test server. The built
// binaries' tests (cmd/immybot-mcp) prove the guard itself.
func init() {
	cliutil.OutboundGuard = func(rt http.RoundTripper) http.RoundTripper { return rt }
	cliutil.HoldValues = func(string, ...string) {}
}
