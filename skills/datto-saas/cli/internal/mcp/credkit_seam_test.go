// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package mcp

import (
	"net/http"
	"os"
	"testing"

	"datto-saas-pp-cli/internal/client"
	"datto-saas-pp-cli/internal/cliutil"
	"datto-saas-pp-cli/internal/credkit"
)

// TestMain: see client.OutboundGuard. No test here runs credkit.Start, and the
// built binaries' leak tests (internal/credwire) prove the guard itself.
func TestMain(m *testing.M) {
	client.OutboundGuard = func(rt http.RoundTripper) http.RoundTripper { return rt }
	cliutil.Hold = func(credkit.Source, ...string) {}
	os.Exit(m.Run())
}
