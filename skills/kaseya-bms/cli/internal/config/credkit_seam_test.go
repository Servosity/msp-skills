// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package config

import (
	"net/http"
)

// No in-process test runs credkit.Start, so the kit's outbound guard would
// refuse every credentialed request to this package's loopback servers. The
// guard is proven on the built binaries (internal/kbmsblackbox).
func init() {
	OutboundGuard = func(rt http.RoundTripper) http.RoundTripper { return rt }
}
