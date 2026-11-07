// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"net/http"

	"appdirect-pp-cli/internal/cliutil"
)

// No in-process test runs credkit.Start, so the outbound guard would refuse
// every credentialed request to a loopback test server. The guard itself is
// proven on the built binaries (internal/credwire kit_classes_test.go).
func init() {
	cliutil.OutboundGuard = func(rt http.RoundTripper) http.RoundTripper {
		if rt == nil {
			return http.DefaultTransport // as credkit.Transport(nil) sends
		}
		return rt
	}
}
