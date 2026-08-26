// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored regression suite for the credential lifecycle commands:
// `auth login` must persist the client-credentials context the client needs to
// re-mint, and `auth set-token` must not carry the previous token's expiry
// forward onto a brand-new token.

package cli

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cipp-pp-cli/internal/config"
)

// TestAuthSetToken_ClearsStaleExpiry pins blocker 3. set-token used to re-save
// cfg.TokenExpiry, so a token pasted into a config whose previous token had
// already expired arrived looking expired. Harmless while nothing read the
// expiry; a live bug the moment the client started gating on it.
func TestAuthSetToken_ClearsStaleExpiry(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	t.Setenv("CIPP_CONFIG", path)
	t.Setenv("CIPP_API_KEY", "")

	seed := &config.Config{Path: path}
	seed.SetOAuthContext("tenant-abc", "", "")
	if err := seed.SaveTokens("cid", "csec", "old-token", "", time.Now().Add(-2*time.Hour)); err != nil {
		t.Fatalf("seed SaveTokens: %v", err)
	}

	flags := &rootFlags{configPath: path}
	cmd := newAuthSetTokenCmd(flags)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"pasted-static-token"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("set-token: %v", err)
	}

	got, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.AccessToken != "pasted-static-token" {
		t.Fatalf("AccessToken = %q", got.AccessToken)
	}
	if !got.TokenExpiry.IsZero() {
		t.Fatalf("set-token carried a stale expiry forward: %s", got.TokenExpiry)
	}
	// set-token replaces an OAuth2 install with a static credential, so the
	// client credentials must be gone: otherwise the re-mint gate would fire
	// against a config the operator explicitly moved off OAuth.
	if got.ClientID != "" || got.ClientSecret != "" {
		t.Fatalf("set-token left client credentials behind: id=%q", got.ClientID)
	}
	if got.AuthHeader() != "Bearer pasted-static-token" {
		t.Fatalf("AuthHeader = %q", got.AuthHeader())
	}
}

// TestAuthLogin_PersistsOAuthContext is blocker 1's other half: --tenant-id,
// --authority and --scope used to be read into local variables and discarded,
// leaving the client with no token URL to refresh against.
func TestAuthLogin_PersistsOAuthContext(t *testing.T) {
	t.Setenv("PRINTING_PRESS_VERIFY", "")
	t.Setenv("PRINTING_PRESS_VERIFY_LIVE_HTTP", "")
	t.Setenv("CIPP_API_KEY", "")

	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"access_token":"login-token","expires_in":3600,"token_type":"Bearer"}`)
	}))
	defer srv.Close()

	path := filepath.Join(t.TempDir(), "config.toml")
	t.Setenv("CIPP_CONFIG", path)

	flags := &rootFlags{configPath: path}
	cmd := newAuthLoginCmd(flags)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{
		"--client-id", "00000000-0000-0000-0000-000000000000",
		"--client-secret", "s3cret",
		"--tenant-id", "11111111-1111-1111-1111-111111111111",
		"--base-url", "https://cipp.example.com/api",
		"--authority", srv.URL,
		"--scope", "api://custom/.default",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("auth login: %v", err)
	}
	if want := "/11111111-1111-1111-1111-111111111111/oauth2/v2.0/token"; gotPath != want {
		t.Fatalf("token endpoint path = %q, want %q", gotPath, want)
	}

	got, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.TenantID != "11111111-1111-1111-1111-111111111111" {
		t.Fatalf("tenant_id not persisted: %q", got.TenantID)
	}
	if got.Authority != srv.URL {
		t.Fatalf("authority not persisted: %q", got.Authority)
	}
	if got.Scope != "api://custom/.default" {
		t.Fatalf("scope not persisted: %q", got.Scope)
	}
	if got.TokenEndpoint() != srv.URL+"/11111111-1111-1111-1111-111111111111/oauth2/v2.0/token" {
		t.Fatalf("stored context does not rebuild the token endpoint: %q", got.TokenEndpoint())
	}
	if !strings.Contains(out.String(), "refreshes automatically") {
		t.Fatalf("login should tell the operator the token now self-refreshes; got:\n%s", out.String())
	}
}
