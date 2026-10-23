// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"nerdio-pp-cli/internal/cli"
)

// TestAuthLoginRefusesATokenRedirectToAnotherOrigin: `auth login` minted
// through http.DefaultClient, which replays the token request body (the client
// secret) on a 307 to another host. A token endpoint that redirects off its
// origin must get an error, and the other host nothing. Hand-owned
// (handfixes.json: token-request-stays-on-origin).
func TestAuthLoginRefusesATokenRedirectToAnotherOrigin(t *testing.T) {
	var elsewhere atomic.Int32
	var leaked atomic.Bool
	second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		elsewhere.Add(1)
		body, _ := io.ReadAll(r.Body)
		if strings.Contains(string(body), "stub-client-secret") {
			leaked.Store(true)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"stub-access-token","token_type":"Bearer","expires_in":3600}`)
	}))
	defer second.Close()
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, second.URL+r.URL.Path, http.StatusTemporaryRedirect)
	}))
	defer first.Close()
	home := t.TempDir()
	for _, k := range []string{"HOME", "USERPROFILE", "XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME", "XDG_CACHE_HOME", "NERDIO_HOME"} {
		t.Setenv(k, home)
	}
	t.Setenv("PRINTING_PRESS_VERIFY", "")
	t.Setenv("NERDIO_NO_CONFIG_WRITE", "")
	t.Setenv("NERDIO_CONFIG", filepath.Join(home, "config.toml"))
	t.Setenv("NERDIO_TOKEN_URL", first.URL+"/oauth/token")
	t.Setenv("NERDIO_CLIENT_ID", "stub-client-id")
	t.Setenv("NERDIO_CLIENT_SECRET", "stub-client-secret")

	cmd := cli.RootCmd()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"auth", "login"})
	err := cmd.Execute()
	if n := elsewhere.Load(); n != 0 {
		t.Fatalf("the second host received %d token request(s) (client secret in the body: %v)", n, leaked.Load())
	}
	if err == nil {
		t.Fatal("auth login reported success although its token endpoint redirected to another origin")
	}
}
