// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): regression guard for handfixes.json entry
// "credkit-wiring" (class secret-echo), through the
// MCP server exactly as main assembles it.

package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestMintedTokenNeverReachesMCPReadResult: the handshake mints a token, then
// the API answers 200 text/html with that token as the page title.
func TestMintedTokenNeverReachesMCPReadResult(t *testing.T) {
	const token = "minted-session-MCP/0123456789+abc"
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1.0/auth" {
			_, _ = w.Write([]byte(token))
			return
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html><title>" + token + "</title></html>"))
	}))
	defer srv.Close()
	bins := t.TempDir()
	mcpBin := buildModuleCommand(t, bins, "avanan-mcp", ".")
	home := t.TempDir()
	var env []string
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if !strings.HasPrefix(name, "AVANAN_") && !strings.HasPrefix(name, "PRINTING_PRESS_") {
			env = append(env, entry)
		}
	}
	env = append(env, "AVANAN_HOME="+home, "AVANAN_CONFIG="+filepath.Join(home, "config.toml"), "AVANAN_BASE_URL="+srv.URL,
		"AVANAN_CA_CERT="+mcpServerCAFile(t, srv), "AVANAN_APP_ID=mcp-runtime-app-0003", "AVANAN_CLIENT_SECRET=mcp-runtime-secret-0003")
	out := callToolOverStdio(t, mcpBin, env, "avanan_read", map[string]any{"endpoint_id": "scopes.get", "params": map[string]any{}})
	if !strings.Contains(out, `"isError":true`) {
		t.Fatalf("avanan_read did not fail on an HTML answer: %s", out)
	}
	assertMCPNoCredential(t, "the avanan_read result", out, token)
}
