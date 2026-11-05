// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package config

import (
	"net/http"

	"quickbooks-pp-cli/internal/cliutil"
	"quickbooks-pp-cli/internal/credkit"
)

// In-process tests run without the credential kit's outbound guard and holds
// (see cliutil.OutboundGuard, cliutil.HoldCredential): none runs credkit.Start,
// so the guard would refuse every credentialed request to the loopback servers
// they stand up, and a held token would be checked against every later test.
// cmd/quickbooks-mcp's child-process tests prove both.
func init() {
	cliutil.OutboundGuard = func(rt http.RoundTripper) http.RoundTripper { return rt }
	cliutil.HoldCredential = func(credkit.Source, ...string) {}
}
