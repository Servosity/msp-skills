// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"

	"sherweb-pp-cli/internal/cli"
)

// TestAuthLoginRefusesATokenRedirectToAnotherOrigin: the token request carries
// the client secret in its body and the APIM subscription key in a header, and
// `auth login` used http.DefaultClient, which replays both on a 307. A token
// endpoint that redirects to another host:port must get an error, and the
// other host nothing. Hand-owned (handfixes.json: redirect-strips-every-credential).
func TestAuthLoginRefusesATokenRedirectToAnotherOrigin(t *testing.T) {
	var elsewhere atomic.Int32
	second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		elsewhere.Add(1)
		_, _ = io.WriteString(w, `{"access_token":"stub-access-token","token_type":"Bearer","expires_in":3600}`)
	}))
	defer second.Close()
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, second.URL+r.URL.Path, http.StatusTemporaryRedirect)
	}))
	defer first.Close()
	home := t.TempDir()
	for _, k := range []string{"HOME", "USERPROFILE", "XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME", "XDG_CACHE_HOME", "SHERWEB_HOME"} {
		t.Setenv(k, home)
	}
	t.Setenv("PRINTING_PRESS_VERIFY", "")
	t.Setenv("SHERWEB_NO_CONFIG_WRITE", "")
	t.Setenv("SHERWEB_CONFIG", filepath.Join(home, "config.toml"))
	t.Setenv("SHERWEB_TOKEN_URL", first.URL+"/auth/oidc/connect/token")
	t.Setenv("SHERWEB_CLIENT_ID", "stub-client-id")
	t.Setenv("SHERWEB_CLIENT_SECRET", "stub-client-secret")
	t.Setenv("SHERWEB_SUBSCRIPTION_KEY", "stub-subscription-key")

	cmd := cli.RootCmd()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"auth", "login"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("auth login followed a token redirect to another origin")
	}
	if n := elsewhere.Load(); n != 0 {
		t.Fatalf("the second host received %d token request(s) carrying the client secret", n)
	}
}
