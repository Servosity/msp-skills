// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored regression suite for `auth login`'s own token exchange.
//
// The client-side re-mint was hardened after an adversarial review found it
// following redirects under Go's default policy, which REPLAYS THE POST BODY on
// 307/308. `auth login` performs the identical exchange, with the identical
// client_credentials form, and it used http.DefaultClient - whose zero
// CheckRedirect IS that default policy. It now installs the same refusal.

package cli

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"cipp-pp-cli/internal/config"
)

// A deliberately low-entropy, stopword-bearing literal: the secret scanner in
// CI flags high-entropy strings next to the word "secret", and a test fixture
// must never look like a real credential.
const loginTestSecret = "not-a-real-secret-example"

func runAuthLogin(t *testing.T, authority, configPath string) (string, error) {
	t.Helper()
	t.Setenv("PRINTING_PRESS_VERIFY", "")
	t.Setenv("PRINTING_PRESS_VERIFY_LIVE_HTTP", "")
	t.Setenv("CIPP_API_KEY", "")
	t.Setenv("CIPP_CONFIG", configPath)

	cmd := newAuthLoginCmd(&rootFlags{configPath: configPath})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SilenceUsage = true
	cmd.SetArgs([]string{
		"--client-id", "00000000-0000-0000-0000-000000000000",
		"--client-secret", loginTestSecret,
		"--tenant-id", "11111111-1111-1111-1111-111111111111",
		"--base-url", "https://cipp.example.com/api",
		"--authority", authority,
	})
	err := cmd.Execute()
	return out.String(), err
}

// TestAuthLogin_RefusesCrossHostRedirect is the fires-on-broken direction. The
// authority 307s to a second host; that host must never see the form, and its
// token must never be cached.
func TestAuthLogin_RefusesCrossHostRedirect(t *testing.T) {
	var mu sync.Mutex
	var bodies []string
	attacker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		mu.Lock()
		bodies = append(bodies, string(body))
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"access_token":"attacker-supplied-token","expires_in":3600}`)
	}))
	defer attacker.Close()
	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, attacker.URL+"/token", http.StatusTemporaryRedirect)
	}))
	defer redirector.Close()

	path := filepath.Join(t.TempDir(), "config.toml")
	out, err := runAuthLogin(t, redirector.URL, path)

	mu.Lock()
	got := len(bodies)
	leaked := false
	for _, b := range bodies {
		if strings.Contains(b, loginTestSecret) {
			leaked = true
		}
	}
	mu.Unlock()

	if got != 0 {
		t.Fatalf("the cross-host redirect target received %d request(s); the client_credentials form was replayed", got)
	}
	if leaked {
		t.Fatal("client_secret reached the redirect target")
	}
	if err == nil {
		t.Fatalf("auth login accepted a cross-host redirect; output:\n%s", out)
	}
	if !strings.Contains(err.Error(), "refusing to resend") {
		t.Fatalf("error should name the refusal; got: %v", err)
	}
	if strings.Contains(err.Error(), loginTestSecret) {
		t.Fatalf("error leaked the client secret: %v", err)
	}
	cfg, loadErr := config.Load(path)
	if loadErr == nil && cfg.AccessToken == "attacker-supplied-token" {
		t.Fatal("the redirect target's token was cached to the config")
	}
}

// TestAuthLogin_PersistsTokenLifetime is the healthy direction plus the
// mint-storm guard: the granted expires_in has to reach the config or the
// client cannot clamp its refresh skew for short-lived tokens.
func TestAuthLogin_PersistsTokenLifetime(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if !strings.Contains(string(body), "grant_type=client_credentials") {
			http.Error(w, "form did not arrive", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"access_token":"login-token","expires_in":120,"token_type":"Bearer"}`)
	}))
	defer srv.Close()

	path := filepath.Join(t.TempDir(), "config.toml")
	out, err := runAuthLogin(t, srv.URL, path)
	if err != nil {
		t.Fatalf("auth login: %v\n%s", err, out)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.TokenLifetime != 120 {
		t.Fatalf("token_lifetime = %d, want 120; the client cannot clamp its skew without it", cfg.TokenLifetime)
	}
	if cfg.AccessToken != "login-token" {
		t.Fatalf("access_token = %q", cfg.AccessToken)
	}
}

// TestAuthLogin_ErrorBodyMasksClientSecret pins the last leak on this path: a
// token endpoint that echoes the submitted form back in its error body.
func TestAuthLogin_ErrorBodyMasksClientSecret(t *testing.T) {
	for _, shape := range []struct {
		name string
		body func(form string) string
	}{
		{"aad json error_description", func(form string) string {
			return fmt.Sprintf(`{"error":"invalid_request","error_description":"rejected: %s"}`, form)
		}},
		{"opaque non-json body", func(form string) string { return "rejected: " + form }},
	} {
		shape := shape
		t.Run(shape.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
				w.WriteHeader(http.StatusBadRequest)
				fmt.Fprint(w, shape.body(string(body)))
			}))
			defer srv.Close()

			_, err := runAuthLogin(t, srv.URL, filepath.Join(t.TempDir(), "config.toml"))
			if err == nil {
				t.Fatal("a 400 from the token endpoint must surface as an error")
			}
			msg := err.Error()
			if strings.Contains(msg, loginTestSecret) {
				t.Fatalf("client_secret printed verbatim:\n%s", msg)
			}
			if esc := url.QueryEscape(loginTestSecret); esc != loginTestSecret && strings.Contains(msg, esc) {
				t.Fatalf("url-encoded client_secret printed:\n%s", msg)
			}
			if !strings.Contains(msg, "****mple") {
				t.Fatalf("the secret should be masked, not deleted:\n%s", msg)
			}
		})
	}
}

// TestMaskSecret is the unit-level both-directions proof for the helper.
func TestMaskSecret(t *testing.T) {
	cases := []struct {
		name   string
		text   string
		secret string
		want   string
	}{
		{"empty secret changes nothing", "client_secret=abc", "", "client_secret=abc"},
		{"empty text changes nothing", "", "abc", ""},
		{"plain occurrence", "client_secret=" + loginTestSecret + "&x=1", loginTestSecret, "client_secret=****mple&x=1"},
		{"url-encoded occurrence", "client_secret=a%2Fb%2Bc", "a/b+c", "client_secret=****/b+c"},
		{"short secret masks entirely", "k=abc", "abc", "k=****"},
		{"absent secret is untouched", "nothing here", loginTestSecret, "nothing here"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if got := maskSecret(tc.text, tc.secret); got != tc.want {
				t.Fatalf("maskSecret = %q, want %q", got, tc.want)
			}
		})
	}
}
