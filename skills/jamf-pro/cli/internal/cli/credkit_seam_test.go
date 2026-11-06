// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"net/http"

	"jamf-pro-pp-cli/internal/client"
	"jamf-pro-pp-cli/internal/cliutil"
)

// This package's in-process tests never run credkit.Start, so the kit's guard
// would refuse their loopback servers, and its held set has no reset. The built
// binaries prove both (internal/jamfblackbox).
func init() {
	client.OutboundGuard = func(rt http.RoundTripper) http.RoundTripper {
		if rt == nil {
			return http.DefaultTransport
		}
		return rt
	}
	cliutil.Hold = func(string, ...string) {}
}
