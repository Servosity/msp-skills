// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"cloudally-pp-cli/internal/client"
	"cloudally-pp-cli/internal/config"
)

// authTrap is a second host. If an auth POST ever follows a redirect here it
// records what it received and answers with a token, so a leak also shows up
// as a saved foreign token.
type authTrap struct {
	srv     *httptest.Server
	hits    atomic.Int32
	mu      sync.Mutex
	secrets []string
	bodies  []string
}

const trapToken = "trap-token"

func newAuthTrap(t *testing.T) *authTrap {
	t.Helper()
	tr := &authTrap{}
	tr.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tr.hits.Add(1)
		raw, _ := io.ReadAll(r.Body)
		tr.mu.Lock()
		tr.secrets = append(tr.secrets, r.Header.Get("client-secret"))
		tr.bodies = append(tr.bodies, string(raw))
		tr.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"accessToken":%q,"tokenType":"Bearer","expiresIn":1893456000000,"refreshToken":"trap-refresh"}`, trapToken)
	}))
	t.Cleanup(tr.srv.Close)
	return tr
}

func (tr *authTrap) assertUntouched(t *testing.T) {
	t.Helper()
	if n := tr.hits.Load(); n != 0 {
		tr.mu.Lock()
		defer tr.mu.Unlock()
		t.Fatalf("the redirect target received %d request(s): client-secret %q, bodies %q", n, tr.secrets, tr.bodies)
	}
}

// redirectingAuthServer answers every request with status, pointing at the
// same path on target.
func redirectingAuthServer(t *testing.T, status int, target string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target+r.URL.Path, status)
	}))
	t.Cleanup(srv.Close)
	return srv
}

var authRedirectStatuses = []int{http.StatusFound, http.StatusTemporaryRedirect, http.StatusPermanentRedirect}

// auth login must not send the email, password or client secret to the host
// a redirect names, and must not save the token that host returns.
func TestAuthLoginRefusesRedirect(t *testing.T) {
	for _, status := range authRedirectStatuses {
		for _, partner := range []bool{true, false} {
			t.Run(fmt.Sprintf("%d/partner=%v", status, partner), func(t *testing.T) {
				isolateAuthState(t)
				trap := newAuthTrap(t)
				auth := redirectingAuthServer(t, status, trap.srv.URL)
				t.Setenv("CLOUDALLY_CLIENT_ID", "cid")
				t.Setenv("CLOUDALLY_CLIENT_SECRET", "csecret")
				t.Setenv("CLOUDALLY_BASE_URL", auth.URL)

				args := []string{"auth", "login", "--email", "tech@example.com", "--password", "secret-pw"}
				if partner {
					args = append(args, "--partner")
				}
				_, _, err := runAuthCmd(t, "", args...)
				if err == nil {
					t.Fatal("login through a redirect succeeded")
				}
				if !strings.Contains(err.Error(), "redirect") {
					t.Fatalf("error should name the redirect: %v", err)
				}
				if strings.Contains(err.Error(), "secret-pw") || strings.Contains(err.Error(), "csecret") {
					t.Fatalf("error leaked a secret: %v", err)
				}
				trap.assertUntouched(t)
				cfg, lerr := config.Load("")
				if lerr != nil {
					t.Fatalf("reload: %v", lerr)
				}
				if cfg.AccessToken != "" || cfg.RefreshToken != "" {
					t.Fatalf("a token was saved: (%q, %q)", cfg.AccessToken, cfg.RefreshToken)
				}
				if actx, ok := loadAuthContext(); ok {
					t.Fatalf("a partner session sidecar was written: %+v", actx)
				}
			})
		}
	}
}

// The automatic refresh sends the refresh token. A redirect must not carry
// it to another host, and the refreshed token must not be saved.
func TestCloudallyRefreshHookRefusesRedirect(t *testing.T) {
	for _, status := range authRedirectStatuses {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			isolateAuthState(t)
			trap := newAuthTrap(t)
			auth := redirectingAuthServer(t, status, trap.srv.URL)

			cfg, err := config.Load("")
			if err != nil {
				t.Fatalf("config.Load: %v", err)
			}
			cfg.BaseURL = auth.URL
			if err := cfg.SaveTokens("cid", "csecret", "old-access", "old-refresh", time.Now().Add(10*time.Second)); err != nil {
				t.Fatalf("seed tokens: %v", err)
			}

			c := client.New(cfg, 5*time.Second, 0)
			if err := cloudallyRefreshHook(c); err == nil {
				t.Fatal("refresh through a redirect succeeded")
			}
			trap.assertUntouched(t)
			if c.Config.AccessToken != "old-access" || c.Config.RefreshToken != "old-refresh" {
				t.Fatalf("in-memory tokens changed: (%q, %q)", c.Config.AccessToken, c.Config.RefreshToken)
			}
			reloaded, err := config.Load("")
			if err != nil {
				t.Fatalf("reload: %v", err)
			}
			if reloaded.AccessToken != "old-access" || reloaded.RefreshToken != "old-refresh" {
				t.Fatalf("saved tokens changed: (%q, %q)", reloaded.AccessToken, reloaded.RefreshToken)
			}
		})
	}
}

// auth use-account sends the partner bearer to the mint endpoint. A redirect
// must not carry it elsewhere, and the foreign token must not become active.
func TestAuthUseAccountRefusesRedirect(t *testing.T) {
	for _, status := range authRedirectStatuses {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			isolateAuthState(t)
			trap := newAuthTrap(t)
			auth := redirectingAuthServer(t, status, trap.srv.URL)

			cfg, err := config.Load("")
			if err != nil {
				t.Fatalf("config.Load: %v", err)
			}
			cfg.BaseURL = auth.URL
			if err := cfg.SaveTokens("cid", "csecret", "partner-token", "partner-refresh", time.Now().Add(time.Hour)); err != nil {
				t.Fatalf("seed tokens: %v", err)
			}
			if err := saveAuthContext(&authContext{Scope: authScopePartner, PartnerAccessToken: "partner-token", PartnerRefreshToken: "partner-refresh"}); err != nil {
				t.Fatalf("seed sidecar: %v", err)
			}

			_, _, err = runAuthCmd(t, "", "auth", "use-account", "5f1c2a")
			if err == nil {
				t.Fatal("use-account through a redirect succeeded")
			}
			trap.assertUntouched(t)
			reloaded, err := config.Load("")
			if err != nil {
				t.Fatalf("reload: %v", err)
			}
			if reloaded.AccessToken != "partner-token" {
				t.Fatalf("active token = %q, want the partner token kept", reloaded.AccessToken)
			}
			if actx, ok := loadAuthContext(); !ok || actx.Scope != authScopePartner {
				t.Fatalf("sidecar = %+v (ok=%v), want partner scope kept", actx, ok)
			}
		})
	}
}

// localhostURL rewrites an httptest URL to use the name localhost: the
// generated client already refuses a cross-origin redirect to an IP literal,
// so a trap on 127.0.0.1 would not prove anything about the body guard.
func localhostURL(raw string) string {
	return strings.Replace(raw, "127.0.0.1", "localhost", 1)
}

// The generated session commands post the password or refresh token in the
// body through the generated client. Its own policy strips the credential
// headers on a cross-origin hop but would still replay the body on a 307 or
// 308; the client hook refuses that.
func TestGeneratedSessionCommandsRefuseBodyRedirect(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"session login", []string{"session", "login", "--email", "tech@example.com", "--password", "secret-pw"}},
		{"session partner-login", []string{"session", "partner-login", "--email", "tech@example.com", "--password", "secret-pw"}},
		{"session refresh", []string{"session", "refresh", "--refresh-token", "secret-refresh"}},
	}
	for _, status := range []int{http.StatusTemporaryRedirect, http.StatusPermanentRedirect} {
		for _, tc := range cases {
			t.Run(fmt.Sprintf("%d/%s", status, tc.name), func(t *testing.T) {
				isolateAuthState(t)
				trap := newAuthTrap(t)
				auth := redirectingAuthServer(t, status, localhostURL(trap.srv.URL))
				t.Setenv("CLOUDALLY_CLIENT_ID", "cid")
				t.Setenv("CLOUDALLY_CLIENT_SECRET", "csecret")
				t.Setenv("CLOUDALLY_ACCESS_TOKEN", "active-token")
				t.Setenv("CLOUDALLY_BASE_URL", auth.URL)

				_, _, err := runAuthCmd(t, "", append(tc.args, "--json")...)
				if err == nil {
					t.Fatal("the command followed a cross-origin redirect")
				}
				trap.assertUntouched(t)
			})
		}
	}
}

// MCP tools build their client with client.New + ApplyClientHooks, so the
// same guard covers them.
func TestClientHooksRefuseBodyRedirectForMCPClients(t *testing.T) {
	isolateAuthState(t)
	trap := newAuthTrap(t)
	auth := redirectingAuthServer(t, http.StatusTemporaryRedirect, localhostURL(trap.srv.URL))
	cfg := &config.Config{BaseURL: auth.URL, ClientID: "cid", ClientSecret: "csecret", AccessToken: "active-token"}
	c := client.New(cfg, 5*time.Second, 0)
	if err := ApplyClientHooks(c); err != nil {
		t.Fatalf("ApplyClientHooks: %v", err)
	}
	if _, _, err := c.Post(context.Background(), "/v2/auth", map[string]string{"email": "tech@example.com", "password": "secret-pw"}); err == nil {
		t.Fatal("POST followed a cross-origin redirect")
	}
	trap.assertUntouched(t)

	// A same-origin redirect is still followed.
	var landed atomic.Int32
	same := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/auth" {
			http.Redirect(w, r, "/v2/auth/moved", http.StatusTemporaryRedirect)
			return
		}
		landed.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer same.Close()
	c2 := client.New(&config.Config{BaseURL: same.URL, ClientID: "cid", ClientSecret: "csecret", AccessToken: "active-token"}, 5*time.Second, 0)
	if err := ApplyClientHooks(c2); err != nil {
		t.Fatalf("ApplyClientHooks: %v", err)
	}
	if _, _, err := c2.Post(context.Background(), "/v2/auth", map[string]string{"k": "v"}); err != nil {
		t.Fatalf("same-origin redirect: %v", err)
	}
	if landed.Load() != 1 {
		t.Fatalf("same-origin target reached %d times, want 1", landed.Load())
	}
}

// A GET (or HEAD) that carries a body is guarded like a POST.
func TestClientHooksRefuseGETBodyRedirect(t *testing.T) {
	isolateAuthState(t)
	trap := newAuthTrap(t)
	auth := redirectingAuthServer(t, http.StatusTemporaryRedirect, localhostURL(trap.srv.URL))
	c := client.New(&config.Config{BaseURL: auth.URL, ClientID: "cid", ClientSecret: "csecret", AccessToken: "active-token"}, 5*time.Second, 0)
	if err := ApplyClientHooks(c); err != nil {
		t.Fatalf("ApplyClientHooks: %v", err)
	}
	req, err := http.NewRequest(http.MethodGet, auth.URL+"/v2/user", strings.NewReader("sensitive-body"))
	if err != nil {
		t.Fatal(err)
	}
	resp, err := c.HTTPClient.Do(req)
	if err == nil {
		resp.Body.Close()
		t.Fatal("a GET with a body followed a cross-origin redirect")
	}
	trap.assertUntouched(t)
}
