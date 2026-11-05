// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"net/http"

	"zoom-phone-pp-cli/internal/cliutil"
	"zoom-phone-pp-cli/internal/credkit"
)

// This package's in-process tests never run credkit.Start, so the kit's guard
// would refuse their loopback servers, and its held set has no reset. The built
// binaries prove both (internal/zoomblackbox).
func init() {
	cliutil.OutboundGuard = func(rt http.RoundTripper) http.RoundTripper { return rt }
	cliutil.Hold = func(credkit.Source, ...string) {}
}
