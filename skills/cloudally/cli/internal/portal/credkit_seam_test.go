// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package portal

import (
	"net/http"
	"os"
	"testing"

	"cloudally-pp-cli/internal/client"
)

// TestMain runs this package's unit tests without the credential kit's outbound
// guard (handfixes.json portal-kit-wiring): no test here runs credkit.Start, so
// the guard would refuse every credentialed request to the loopback servers
// these tests stand up. The built binaries' leak tests (internal/credwire) prove
// the guard on the portal session.
func TestMain(m *testing.M) {
	client.OutboundGuard = func(rt http.RoundTripper) http.RoundTripper { return rt }
	client.HoldMinted = func(string, ...string) {}
	os.Exit(m.Run())
}
