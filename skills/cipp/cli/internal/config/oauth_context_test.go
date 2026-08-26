// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored regression suite for the OAuth2 client-credentials context
// (tenant_id / authority / scope) that `auth login` persists so the client can
// re-mint an expired token. Before these fields existed the tenant was
// accepted as a flag and thrown away.

package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTokenEndpoint(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		cfg  *Config
		want string
	}{
		{"no tenant means no endpoint", &Config{}, ""},
		{"nil config", nil, ""},
		{"default authority", &Config{TenantID: "tid"}, "https://login.microsoftonline.com/tid/oauth2/v2.0/token"},
		{"custom sovereign authority", &Config{TenantID: "tid", Authority: "https://login.microsoftonline.us"}, "https://login.microsoftonline.us/tid/oauth2/v2.0/token"},
		{"trailing slash trimmed", &Config{TenantID: "tid", Authority: "https://login.microsoftonline.com/"}, "https://login.microsoftonline.com/tid/oauth2/v2.0/token"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.cfg.TokenEndpoint(); got != tc.want {
				t.Fatalf("TokenEndpoint() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestOAuthScope(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		cfg  *Config
		want string
	}{
		{"stored scope wins", &Config{Scope: "api://custom/.default", ClientID: "cid"}, "api://custom/.default"},
		{"derived from client id", &Config{ClientID: "cid"}, "api://cid/.default"},
		{"nothing to derive from", &Config{}, ""},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.cfg.OAuthScope(); got != tc.want {
				t.Fatalf("OAuthScope() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestSetOAuthContextRoundTrips proves the fields actually reach disk and come
// back through Load. A re-mint reads them from a FRESH process, so an
// in-memory-only assignment would be worthless.
func TestSetOAuthContextRoundTrips(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	t.Setenv("CIPP_CONFIG", path)
	t.Setenv("CIPP_API_KEY", "")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	cfg.BaseURL = "https://cipp.example.com/api"
	cfg.SetOAuthContext("tenant-abc", "https://login.microsoftonline.us", "api://custom/.default")
	expiry := time.Now().Add(time.Hour).UTC().Truncate(time.Second)
	if err := cfg.SaveTokens("cid", "csec", "tok", "", expiry); err != nil {
		t.Fatalf("SaveTokens: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	for _, want := range []string{"tenant_id", "tenant-abc", "authority", "scope"} {
		if !strings.Contains(string(raw), want) {
			t.Fatalf("config.toml missing %q:\n%s", want, raw)
		}
	}

	reloaded, err := Load(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.TenantID != "tenant-abc" {
		t.Fatalf("TenantID = %q", reloaded.TenantID)
	}
	if reloaded.Authority != "https://login.microsoftonline.us" {
		t.Fatalf("Authority = %q", reloaded.Authority)
	}
	if reloaded.Scope != "api://custom/.default" {
		t.Fatalf("Scope = %q", reloaded.Scope)
	}
	if got := reloaded.TokenEndpoint(); got != "https://login.microsoftonline.us/tenant-abc/oauth2/v2.0/token" {
		t.Fatalf("TokenEndpoint after reload = %q", got)
	}
}

// TestClearTokensLeavesNoRemintPath is the logout contract: after logout the
// tenant metadata may survive (it is public, like base_url) but the client
// credentials must not, so nothing can mint a new token unattended.
func TestClearTokensLeavesNoRemintPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	cfg := &Config{Path: path}
	cfg.SetOAuthContext("tenant-abc", "", "")
	if err := cfg.SaveTokens("cid", "csec", "tok", "", time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("SaveTokens: %v", err)
	}
	if err := cfg.ClearTokens(); err != nil {
		t.Fatalf("ClearTokens: %v", err)
	}
	if cfg.ClientID != "" || cfg.ClientSecret != "" {
		t.Fatalf("logout left client credentials behind: id=%q secret set=%v", cfg.ClientID, cfg.ClientSecret != "")
	}
	if cfg.AccessToken != "" || !cfg.TokenExpiry.IsZero() {
		t.Fatalf("logout left a token behind")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if strings.Contains(string(raw), "csec") {
		t.Fatalf("client secret still on disk after logout:\n%s", raw)
	}
}
