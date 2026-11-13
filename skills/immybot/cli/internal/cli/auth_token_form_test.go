// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"sync"
	"testing"

	"immybot-pp-cli/internal/cli"
)

// TestAuthLoginSendsTheClientPairInTheForm: `auth login` sends client_id and
// client_secret in the form body, grant_type first, never as HTTP Basic, the
// same as the client's automatic mint (handfixes.json entra-token-form-auth,
// issue #698; credwire's TestRenewalMintIsNotWithheld is the kit-level proof).
func TestAuthLoginSendsTheClientPairInTheForm(t *testing.T) {
	var mu sync.Mutex
	var bodies []string
	var basic []bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_, _, ok := r.BasicAuth()
		mu.Lock()
		bodies = append(bodies, string(raw))
		basic = append(basic, ok)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"stub-access-token","token_type":"Bearer","expires_in":3600}`)
	}))
	defer srv.Close()
	home := t.TempDir()
	for _, k := range []string{"HOME", "USERPROFILE", "XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME", "XDG_CACHE_HOME", "IMMYBOT_HOME"} {
		t.Setenv(k, home)
	}
	t.Setenv("PRINTING_PRESS_VERIFY", "")
	t.Setenv("IMMYBOT_NO_CONFIG_WRITE", "")
	t.Setenv("IMMYBOT_CONFIG", filepath.Join(home, "config.toml"))
	t.Setenv("IMMYBOT_TOKEN_URL", srv.URL+"/token")
	t.Setenv("IMMYBOT_OAUTH_SCOPE", "api://stub-client-id/.default")
	t.Setenv("IMMYBOT_CLIENT_ID", "stub-client-id")
	t.Setenv("IMMYBOT_CLIENT_SECRET", "stub-client-secret")

	cmd := cli.RootCmd()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"auth", "login"})
	_ = cmd.Execute()

	mu.Lock()
	defer mu.Unlock()
	if len(bodies) != 1 {
		t.Fatalf("auth login sent %d token request(s), want 1", len(bodies))
	}
	if basic[0] {
		t.Errorf("auth login sent the client pair as HTTP Basic")
	}
	const first = "grant_type=client_credentials&"
	if len(bodies[0]) < len(first) || bodies[0][:len(first)] != first {
		t.Errorf("token form %q does not start with %q", bodies[0], first)
	}
	form, err := url.ParseQuery(bodies[0])
	if err != nil {
		t.Fatalf("token form %q: %v", bodies[0], err)
	}
	if form.Get("client_id") != "stub-client-id" || form.Get("client_secret") != "stub-client-secret" {
		t.Errorf("token form %q does not carry the client pair", bodies[0])
	}
}
