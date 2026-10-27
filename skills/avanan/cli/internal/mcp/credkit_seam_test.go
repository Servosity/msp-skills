// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package mcp

import (
	"net/http"

	"avanan-pp-cli/internal/client"
	"avanan-pp-cli/internal/cliutil"
)

// In-process tests run no credkit.Start: the kit's outbound guard would refuse
// every credentialed request to their loopback servers, and their fixtures would
// stay held for every later test. A test that holds a value runs alone
// (heldChild). The guard and the holds are proven on the built binaries
// (internal/cli and cmd/avanan-mcp credential tests).
func init() {
	client.OutboundGuard = func(rt http.RoundTripper) http.RoundTripper { return rt }
	if !inHeldChild() {
		cliutil.Hold = func(string, ...string) {}
	}
}
