// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/dnsfilter/handfixes.json
// under the "mcp-auth-errors-keep-remedy" entry.

package mcp

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"dnsfilter-pp-cli/internal/cliutil"
	"dnsfilter-pp-cli/internal/mcp/redact"
)

// TestReadOnlyAuthErrorsKeepRemedy: under the read-only switch a mirrored
// tool's 401 or 403 keeps the HTTP status and the remedy (the CLI's hint
// export DNSFILTER_API_KEY="your-token-here", a documentation placeholder the
// store's text rule exempts by exact value), while the same hint carrying a
// real-looking key value is still withheld.
func TestReadOnlyAuthErrorsKeepRemedy(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(srv.Close)
	setIsolatedHome(t, srv.URL)
	t.Setenv("DNSFILTER_CLI_PATH", os.Args[0])
	t.Setenv(cliutil.MCPReadOnlyEnvVar, "1")
	tools := map[string]map[string]any{
		"tail":            {"resource": "networks", "follow": false},
		"unblocks_triage": {},
		"policy_audit":    {},
	}
	for _, mode := range []string{"auth401", "auth403"} {
		t.Setenv(fakeCLIModeEnv, mode)
		s := newRegisteredServer(t)
		status := "HTTP 401"
		if mode == "auth403" {
			status = "HTTP 403"
		}
		for tool, args := range tools {
			text := strings.Join(resultTexts(t, callToolWire(t, s, tool, args)), "\n")
			if strings.Contains(text, "withheld: this result") {
				t.Fatalf("%s %s: read-only withheld the auth error: %.300s", tool, mode, text)
			}
			for _, want := range []string{status, `export DNSFILTER_API_KEY="your-token-here"`, `$env:DNSFILTER_API_KEY = "your-token-here"`, "help.dnsfilter.com"} {
				if !strings.Contains(text, want) {
					t.Fatalf("%s %s: missing %q: %.500s", tool, mode, want, text)
				}
			}
		}
	}
	t.Setenv(fakeCLIModeEnv, "auth-real-key")
	s := newRegisteredServer(t)
	for tool, args := range tools {
		frame := callToolWire(t, s, tool, args)
		if strings.Contains(frame, "9f8e7d6c5b4a") {
			t.Fatalf("%s: a real-looking key in the hint reached the client: %.400s", tool, frame)
		}
		if !strings.Contains(frame, strings.SplitN(redact.WithheldBlockMessage, ";", 2)[0]) {
			t.Fatalf("%s: the real-key hint was not withheld: %.400s", tool, frame)
		}
	}
}
