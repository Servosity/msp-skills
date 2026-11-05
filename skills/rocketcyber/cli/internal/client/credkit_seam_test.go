// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"net/http"
	"os"
	"testing"

	"rocketcyber-pp-cli/internal/cliutil"
	"rocketcyber-pp-cli/internal/credkit"
)

// TestMain runs this package's unit tests without the credential kit's outbound
// guard (handfixes.json credkit-wiring): no test here runs credkit.Start, so the
// guard would refuse every credentialed request to the loopback servers these
// tests stand up. The guard is proven on the built binaries by the leak tests in
// internal/credwire.
func TestMain(m *testing.M) {
	OutboundGuard = func(rt http.RoundTripper) http.RoundTripper { return rt }
	cliutil.Hold = func(credkit.Source, ...string) {}
	os.Exit(m.Run())
}
