// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"net/http"
)

// In-process tests run no credkit.Start, so the kit's outbound guard would refuse
// every credentialed request to the loopback servers they stand up. The guard is
// proven on the built binaries (internal/cli and cmd/avanan-mcp credential tests).
func init() {
	OutboundGuard = func(rt http.RoundTripper) http.RoundTripper { return rt }
}
