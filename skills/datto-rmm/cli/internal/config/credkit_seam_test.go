// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package config

import (
	"net/http"
	"os"
	"testing"

	"datto-rmm-pp-cli/internal/cliutil"
	"datto-rmm-pp-cli/internal/credkit"
)

// TestMain runs this package's unit tests without the credential kit's outbound
// guard (handfixes.json credkit-wiring): no test here runs credkit.Start, so the
// guard would refuse every credentialed request to the loopback servers these
// tests stand up. The guard is proven on the built binaries in internal/credwire.
func TestMain(m *testing.M) {
	OutboundGuard = func(rt http.RoundTripper) http.RoundTripper { return rt }
	cliutil.Hold = func(credkit.Source, ...string) {}
	cliutil.HoldBody = func([]byte) {}
	os.Exit(m.Run())
}
