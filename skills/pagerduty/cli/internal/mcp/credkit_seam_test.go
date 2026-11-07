// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package mcp

import (
	"net/http"

	"pagerduty-pp-cli/internal/client"
)

// No in-process test runs credkit.Start, so the outbound guard would refuse
// every credentialed request and then fail closed for the whole test process.
// The built binaries' tests (internal/credwire) prove it.
func init() {
	client.OutboundGuard = func(rt http.RoundTripper) http.RoundTripper { return rt }
}
