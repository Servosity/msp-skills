// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-fix (handfixes.json vault-secrets-withheld): end to end through the
// real MCP read handler. Before the filter, hudu_read with
// {"endpoint_id":"asset-passwords.get","params":{"id":"7"}} handed the agent
// the entry's password and TOTP seed, and left them in the response cache.

package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	mcplib "github.com/mark3labs/mcp-go/mcp"
)

func TestVaultSecretsWithheldFromMCPRead(t *testing.T) {
	const pw, otp = "SENTINEL-PW-1", "SENTINEL-OTP-1"
	entry := `{"id":7,"company_id":3,"name":"DC admin","username":"administrator","password":"` + pw + `","otp_secret":"` + otp + `","updated_at":"2026-02-03T04:05:06Z"}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/asset_passwords/7":
			_, _ = w.Write([]byte(`{"asset_password":` + entry + `}`))
		case "/asset_passwords":
			_, _ = w.Write([]byte(`{"asset_passwords":[` + entry + `]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"not found"}`))
		}
	}))
	defer srv.Close()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("HUDU_HOME", home)
	t.Setenv("HUDU_BASE_URL", srv.URL)
	t.Setenv("HUDU_API_KEY", "fixture-token")
	t.Setenv("HUDU_ACCESS_TOKEN", "fixture-token")
	t.Setenv("HUDU_CLIENT_ID", "")
	t.Setenv("HUDU_CLIENT_SECRET", "")
	t.Setenv("HUDU_NO_LOCAL_DATA", "")
	t.Setenv("PRINTING_PRESS_CLIENT_PROFILE", "")
	t.Setenv("PRINTING_PRESS_VERIFY", "")
	t.Setenv("PRINTING_PRESS_VERIFY_LIVE_HTTP", "")
	cfg := filepath.Join(home, ".config", "hudu-cli", "config.toml")
	if err := os.MkdirAll(filepath.Dir(cfg), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg, []byte("access_token = \"fixture-token\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HUDU_CONFIG", cfg)

	for _, tc := range []struct {
		id     string
		params map[string]any
	}{
		{"asset-passwords.get", map[string]any{"id": "7"}},
		{"asset-passwords.list", map[string]any{}},
		// A second get is served from the response cache.
		{"asset-passwords.get", map[string]any{"id": "7"}},
	} {
		req := mcplib.CallToolRequest{}
		req.Params.Arguments = map[string]any{"endpoint_id": tc.id, "params": tc.params}
		result, err := handleCodeOrchRead(context.Background(), req)
		if err != nil {
			t.Fatalf("%s: %v", tc.id, err)
		}
		if result == nil || result.IsError {
			t.Fatalf("%s: read failed: %#v", tc.id, result)
		}
		out, _ := json.Marshal(result)
		if strings.Contains(string(out), pw) || strings.Contains(string(out), otp) {
			t.Fatalf("%s handed the agent a vault secret: %s", tc.id, out)
		}
		if !strings.Contains(string(out), "administrator") {
			t.Fatalf("%s lost the username metadata: %s", tc.id, out)
		}
	}

	// Nothing under HOME (response cache, local mirror, journal) holds it.
	_ = filepath.Walk(home, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		b, rerr := os.ReadFile(p)
		if rerr == nil && (strings.Contains(string(b), pw) || strings.Contains(string(b), otp)) {
			t.Fatalf("file %s holds a vault secret", p)
		}
		return nil
	})
}
