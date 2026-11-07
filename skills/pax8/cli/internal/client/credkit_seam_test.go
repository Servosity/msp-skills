// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"net/http"
	"os"
	"testing"

	"pax8-pp-cli/internal/cliutil"
	"pax8-pp-cli/internal/credkit"
)

// TestMain runs this package's unit tests without the credential kit's outbound
// guard (handfixes.json credkit-wiring): no test here runs credkit.Start, so the
// guard would refuse every credentialed request to the loopback servers these
// tests stand up. The guard is proven on the built binaries by
// internal/pax8blackbox.
func TestMain(m *testing.M) {
	OutboundGuard = func(rt http.RoundTripper) http.RoundTripper { return rt }
	cliutil.Hold = func(credkit.Source, ...string) {}
	os.Exit(m.Run())
}
