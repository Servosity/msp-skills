// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// api-origin-pinned, the two pins (appdirect_guard.go): any request may go to
// the API base URL's origin; the token URL's origin, when it differs, takes
// only POST <exact token path> carrying no Authorization or HTTP Basic (the
// client pair), never a bearer token, never an API request.
func TestTokenHostTakesOnlyTheTokenPost(t *testing.T) {
	pins := newAppDirectPins("https://reseller.example.com/api", "https://login.example.net/oauth2/token")
	basic := "Basic " + base64.StdEncoding.EncodeToString([]byte("client-id:client-secret"))
	for _, c := range []struct {
		name, method, url, auth string
		ok                      bool
	}{
		{"API read on the API origin", http.MethodGet, "https://reseller.example.com/api/account/v2/companies", "Bearer minted", true},
		{"API write on the API origin", http.MethodPatch, "https://RESELLER.example.com:443/api/account/v2/companies/co-1", "Bearer minted", true},
		{"token POST with Basic", http.MethodPost, "https://login.example.net/oauth2/token", basic, true},
		{"token POST with no Authorization", http.MethodPost, "https://login.example.net/oauth2/token", "", true},
		{"bearer token to the token host", http.MethodPost, "https://login.example.net/oauth2/token", "Bearer minted", false},
		{"any other scheme to the token host", http.MethodPost, "https://login.example.net/oauth2/token", "Digest x", false},
		{"API read sent to the token host", http.MethodGet, "https://login.example.net/api/account/v2/companies", "Bearer minted", false},
		{"API write sent to the token host with Basic", http.MethodPost, "https://login.example.net/api/account/v2/companies", basic, false},
		{"GET of the token path", http.MethodGet, "https://login.example.net/oauth2/token", basic, false},
		{"another path on the token host", http.MethodPost, "https://login.example.net/oauth2/token/extra", basic, false},
		{"token host over plain HTTP", http.MethodPost, "http://login.example.net/oauth2/token", basic, false},
		{"token host on another port", http.MethodPost, "https://login.example.net:8443/oauth2/token", basic, false},
		{"AppDirect's own marketplace", http.MethodPost, "https://marketplace.appdirect.com/oauth2/token", basic, false},
		{"another host", http.MethodGet, "https://evil.example/api/account/v2/companies", "Bearer minted", false},
	} {
		req, err := http.NewRequest(c.method, c.url, nil)
		if err != nil {
			t.Fatal(err)
		}
		if c.auth != "" {
			req.Header.Set("Authorization", c.auth)
		}
		if err := pins.check(req); (err == nil) != c.ok {
			t.Errorf("%s (%s %s): allowed=%v, want %v (%v)", c.name, c.method, c.url, err == nil, c.ok, err)
		}
	}
	// A token URL carrying userinfo pins no token host at all.
	userinfo := newAppDirectPins("https://reseller.example.com/api", "https://u:p@login.example.net/oauth2/token")
	req, _ := http.NewRequest(http.MethodPost, "https://login.example.net/oauth2/token", nil)
	req.Header.Set("Authorization", basic)
	if userinfo.check(req) == nil {
		t.Error("a token URL with userinfo must pin no token host")
	}
}

// The same rule on a live client: with APPDIRECT_TOKEN_URL on another origin,
// the mint posts the pair with HTTP Basic to that exact path and the API
// request carries the minted bearer to the API origin; an API request aimed
// at the token host is refused before it is sent; and a redirect hop must stay
// on the origin of the request it follows, even to the other pinned origin.
func TestGuardSendsOnlyTheTokenPostToTheTokenHost(t *testing.T) {
	const id, secret, minted = "guardClientId-4c1e", "guardClientSecret-8a3d6f", "guardMinted-2b7e9d"
	var tokenHost struct {
		mints, other atomic.Int32
		saw          atomic.Value
	}
	token := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotID, gotSecret, ok := r.BasicAuth()
		raw, _ := io.ReadAll(r.Body)
		form, _ := url.ParseQuery(string(raw))
		if r.Method != http.MethodPost || r.URL.Path != "/oauth2/token" || !ok || gotID != id || gotSecret != secret || form.Get("grant_type") != "client_credentials" {
			tokenHost.other.Add(1)
			tokenHost.saw.Store(r.Method + " " + r.URL.Path + " " + r.Header.Get("Authorization"))
			w.WriteHeader(http.StatusForbidden)
			return
		}
		tokenHost.mints.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"`+minted+`","expires_in":3600}`)
	}))
	t.Cleanup(token.Close)
	var apiAuth atomic.Value
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiAuth.Store(r.Header.Get("Authorization"))
		if r.URL.Path == "/api/redirect-to-token-host" {
			http.Redirect(w, r, token.URL+"/oauth2/token", http.StatusTemporaryRedirect)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `[]`)
	}))
	t.Cleanup(api.Close)
	credRulesHome(t)
	t.Setenv("APPDIRECT_BASE_URL", api.URL+"/api")
	t.Setenv("APPDIRECT_TOKEN_URL", token.URL+"/oauth2/token")
	t.Setenv("APPDIRECT_CLIENT_ID", id)
	t.Setenv("APPDIRECT_CLIENT_SECRET", secret)
	t.Setenv("APPDIRECT_NO_CONFIG_WRITE", "1")
	flags := rootFlags{timeout: 10 * time.Second}
	c, err := flags.newClient()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Get(context.Background(), "/billing/v1/subscriptions", nil); err != nil {
		t.Fatalf("read through the token host's mint: %v", err)
	}
	if tokenHost.mints.Load() != 1 || apiAuth.Load() != "Bearer "+minted {
		t.Fatalf("want one mint at the token host and the minted bearer on the API origin; mints %d, API saw %v", tokenHost.mints.Load(), apiAuth.Load())
	}
	for _, probe := range []struct{ method, url, auth string }{
		{http.MethodGet, token.URL + "/api/billing/v1/subscriptions", "Bearer " + minted},
		{http.MethodPost, token.URL + "/oauth2/token", "Bearer " + minted},
		{http.MethodGet, token.URL + "/oauth2/token", ""},
	} {
		req, _ := http.NewRequest(probe.method, probe.url, nil)
		if probe.auth != "" {
			req.Header.Set("Authorization", probe.auth)
		}
		resp, err := c.HTTPClient.Do(req)
		if resp != nil {
			_ = resp.Body.Close()
		}
		if err == nil || !strings.Contains(err.Error(), "leave the configured AppDirect host") {
			t.Errorf("%s %s (%q) on the token host: want the pin refusal, got %v", probe.method, probe.url, probe.auth, err)
		}
	}
	if _, err := c.Get(context.Background(), "/redirect-to-token-host", nil); err == nil || !strings.Contains(err.Error(), "leave the configured AppDirect host") {
		t.Errorf("an API read redirected to the token host: want the pin refusal, got %v", err)
	}
	// A 307 on an API write resends its method and body; to the token URL
	// the hop is a POST of the exact token path with no Authorization (the
	// transport drops it across hosts), which the token pin alone would let
	// through, carrying the API body to the token host. The hop must stay on
	// the origin of the request it follows.
	if _, _, err := c.Post(context.Background(), "/redirect-to-token-host", map[string]any{"name": "API body for the API host only"}); err == nil || !strings.Contains(err.Error(), "leave the configured AppDirect host") {
		t.Errorf("an API write redirected to the token host: want the pin refusal, got %v", err)
	}
	if n := tokenHost.other.Load(); n != 0 {
		t.Fatalf("the token host received %d request(s) that were not the token POST (last: %v)", n, tokenHost.saw.Load())
	}
}

// auth login with only APPDIRECT_BASE_URL set to a reseller marketplace mints
// at that marketplace's /oauth2/token (client.ResolveAppDirectTokenURL): HTTP
// Basic, the documented form, and the token lands in the 0600 credential
// store, never config.toml. 4.34.2 hard-coded AppDirect's own marketplace as
// the token URL, so the reseller's client secret went to
// marketplace.appdirect.com (appdirect handfix per-marketplace-token-url).
// TestAuthLoginMintsOnlyAtItsMarketplace (appdirectblackbox) proves on the
// built binary, through a recording proxy, that nothing leaves for another host.
func TestAuthLoginMintsAtTheConfiguredMarketplace(t *testing.T) {
	const id, secret, minted = "loginClientId-7e2c", "loginClientSecret-3f9a1b", "loginMinted-5d8c2e"
	m := newFakeMarketplace(t, id, secret, minted, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	credRulesHome(t)
	home := os.Getenv("APPDIRECT_HOME")
	t.Setenv("APPDIRECT_BASE_URL", m.srv.URL+"/api")
	t.Setenv("APPDIRECT_CLIENT_ID", id)
	t.Setenv("APPDIRECT_CLIENT_SECRET", secret)
	var out bytes.Buffer
	cmd := newRootCmd(&rootFlags{})
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"auth", "login"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("auth login against the configured marketplace: %v\n%s", err, out.String())
	}
	if m.mints.Load() != 1 || m.bad.Load() != 0 {
		t.Fatalf("want one accepted mint at %s/oauth2/token, got %d (refused %d; last %v)", m.srv.URL, m.mints.Load(), m.bad.Load(), m.lastMintForm.Load())
	}
	if !strings.Contains(out.String(), "Logged in") || strings.Contains(out.String(), minted) || strings.Contains(out.String(), secret) {
		t.Errorf("auth login output:\n%s", out.String())
	}
	var sawToken bool
	_ = filepath.WalkDir(home, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		b, _ := os.ReadFile(path)
		if strings.HasPrefix(d.Name(), "credentials") {
			sawToken = sawToken || bytes.Contains(b, []byte(minted))
			return nil
		}
		for _, v := range []string{minted, secret} {
			if bytes.Contains(b, []byte(v)) {
				t.Errorf("%s holds %q: only the credential store may", path, v)
			}
		}
		return nil
	})
	if !sawToken {
		t.Errorf("auth login stored no token in the credential store")
	}
}

// Redirect hops must stay on the origin of the request they follow, even when
// the target is the other pinned origin: a 307 on an API write to the token
// URL is a bodied POST of the exact token path with its Authorization dropped,
// which the token pin alone accepts. The check runs on the installed redirect
// policy itself, with an IP-literal token host so no name is resolved.
func TestRedirectHopStaysOnItsOrigin(t *testing.T) {
	const base, tokenURL = "https://reseller.example.com/api", "https://203.0.113.10/oauth2/token"
	hc := &http.Client{}
	installPins(hc, newAppDirectPins(base, tokenURL), false)
	hop := func(fromMethod, from, to string) error {
		via, _ := http.NewRequest(fromMethod, from, nil)
		req, _ := http.NewRequest(fromMethod, to, nil)
		return hc.CheckRedirect(req, []*http.Request{via})
	}
	if err := hop(http.MethodPost, base+"/account/v2/companies", tokenURL); !errors.Is(err, ErrOriginChanged) {
		t.Errorf("an API write redirected to the token URL: want ErrOriginChanged, got %v", err)
	}
	if err := hop(http.MethodPost, tokenURL, base+"/account/v2/companies"); !errors.Is(err, ErrOriginChanged) {
		t.Errorf("the token request redirected to the API origin: want ErrOriginChanged, got %v", err)
	}
	if err := hop(http.MethodGet, base+"/account/v2/companies", "https://evil.example/api/account/v2/companies"); !errors.Is(err, ErrOriginChanged) {
		t.Errorf("an API read redirected to another host: want ErrOriginChanged, got %v", err)
	}
	if err := hop(http.MethodGet, base+"/account/v2/companies", base+"/account/v2/companies?page=1"); err != nil {
		t.Errorf("a same-origin hop must be followed: %v", err)
	}
}
