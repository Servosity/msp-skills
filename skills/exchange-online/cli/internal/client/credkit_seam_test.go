// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"net/http"
	"os"
	"testing"

	"exchange-online-pp-cli/internal/cliutil"
	"exchange-online-pp-cli/internal/credkit"
)

// TestMain runs this package's unit tests without the credential kit's outbound
// guard and hold (see cliutil.OutboundGuard and cliutil.Hold): no test here runs
// credkit.Start, so the guard would refuse every credentialed request to the
// loopback servers these tests stand up, and a value held here would stay held for
// the whole test process. Both are proven on the built binaries by the leak tests in
// internal/exoblackbox.
func TestMain(m *testing.M) {
	cliutil.OutboundGuard = func(rt http.RoundTripper) http.RoundTripper { return rt }
	cliutil.Hold = func(credkit.Source, ...string) {}
	os.Exit(m.Run())
}
