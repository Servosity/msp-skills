// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"net/http"

	"rewst-pp-cli/internal/client"
)

// This package's unit tests run without the credential kit's outbound guard
// (see client.OutboundGuard): no test here runs credkit.Start, so the guard
// would refuse every credentialed request to the loopback fakes these tests
// stand up. The guard is proven on the built binaries by the leak tests in
// internal/rewstblackbox.
//
// The hand-written tests that hold a credential in this process sort last, as
// zz_rewst_*_test.go: go test runs files in name order, and once a value is held
// every later store open costs the kit about 2 s (316 CREATE statements checked).
// Sorted first, they took this package from about 80 s to 400 s and past go test's
// 10-minute timeout on a slow Linux row.
func init() {
	client.OutboundGuard = func(rt http.RoundTripper) http.RoundTripper { return rt }
}
