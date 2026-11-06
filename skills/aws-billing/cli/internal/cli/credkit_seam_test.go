// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"net/http"
	"os"

	"aws-billing-pp-cli/internal/awsx"
	"aws-billing-pp-cli/internal/client"
	"aws-billing-pp-cli/internal/credkit"
)

// No in-process test runs credkit.Start, so the outbound guard would refuse
// every credentialed request to a loopback test server and then fail closed
// for the whole test process, and a credential an AWS command resolves would
// stay held for every later test. A test that holds a value runs alone
// (inFreshProcess). The built binaries' tests (internal/credwire,
// internal/awsbillingblackbox) prove the guard and the holds.
func init() {
	identity := func(rt http.RoundTripper) http.RoundTripper { return rt }
	client.OutboundGuard = identity
	awsx.OutboundGuard = identity
	if os.Getenv("AWS_BILLING_TEST_FRESH_PROCESS") == "" {
		awsx.HoldMinted = func(credkit.Source, ...string) {}
	}
}
