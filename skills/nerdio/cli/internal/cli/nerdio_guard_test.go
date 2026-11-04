// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// guardEntraToken is the operator's Entra token URL the pin tests use.
const guardEntraToken = "https://login.microsoftonline.com/7c1e9a3f-2b4d-4e6a-8c1f-3a5b7d9e1f2a/oauth2/v2.0/token"

// api-origin-pinned, the two pins (nerdio_guard.go): any request may go to
// the NMM instance's origin; the Entra token URL's origin takes only POST
// <exact token path> carrying no Authorization or HTTP Basic (the client
// pair), never a bearer token, never an API request.
func TestTokenHostTakesOnlyTheTokenPost(t *testing.T) {
	pins := newNerdioPins("https://nmm.contoso.com", guardEntraToken)
	basic := "Basic " + base64.StdEncoding.EncodeToString([]byte("client-id:client-secret"))
	tenantPath := "https://login.microsoftonline.com/7c1e9a3f-2b4d-4e6a-8c1f-3a5b7d9e1f2a"
	for _, c := range []struct {
		name, method, url, auth string
		ok                      bool
	}{
		{"API read on the API origin", http.MethodGet, "https://nmm.contoso.com/rest-api/v1/accounts", "Bearer minted", true},
		{"API write on the API origin", http.MethodPut, "https://NMM.contoso.com:443/rest-api/v1/accounts/101/reservations/r-1", "Bearer minted", true},
		{"token POST with Basic", http.MethodPost, guardEntraToken, basic, true},
		{"token POST with no Authorization", http.MethodPost, guardEntraToken, "", true},
		{"bearer token to the token host", http.MethodPost, guardEntraToken, "Bearer minted", false},
		{"any other scheme to the token host", http.MethodPost, guardEntraToken, "Digest x", false},
		{"API read sent to the token host", http.MethodGet, "https://login.microsoftonline.com/rest-api/v1/accounts", "Bearer minted", false},
		{"API write sent to the token host with Basic", http.MethodPost, "https://login.microsoftonline.com/rest-api/v1/accounts/101/host-pool", basic, false},
		{"GET of the token path", http.MethodGet, guardEntraToken, basic, false},
		{"another path on the token host", http.MethodPost, tenantPath + "/oauth2/v2.0/devicecode", basic, false},
		{"another tenant on the token host", http.MethodPost, "https://login.microsoftonline.com/other-tenant/oauth2/v2.0/token", basic, false},
		{"token host over plain HTTP", http.MethodPost, "http://login.microsoftonline.com/7c1e9a3f-2b4d-4e6a-8c1f-3a5b7d9e1f2a/oauth2/v2.0/token", basic, false},
		{"token host on another port", http.MethodPost, "https://login.microsoftonline.com:8443/7c1e9a3f-2b4d-4e6a-8c1f-3a5b7d9e1f2a/oauth2/v2.0/token", basic, false},
		{"the shipped placeholder tenant", http.MethodPost, "https://login.microsoftonline.com/YOUR_TENANT_ID/oauth2/v2.0/token", basic, false},
		{"another host", http.MethodGet, "https://evil.example/rest-api/v1/accounts", "Bearer minted", false},
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
	userinfo := newNerdioPins("https://nmm.contoso.com", "https://u:p@login.microsoftonline.com/7c1e9a3f-2b4d-4e6a-8c1f-3a5b7d9e1f2a/oauth2/v2.0/token")
	req, _ := http.NewRequest(http.MethodPost, guardEntraToken, nil)
	req.Header.Set("Authorization", basic)
	if userinfo.check(req) == nil {
		t.Error("a token URL with userinfo must pin no token host")
	}
}

// The shipped base_url (https://nmm.example.com) is a placeholder, no NMM
// instance of anyone's: the guard pins nothing for it, so every API request
// to it is refused and the refusal names NERDIO_BASE_URL; the token POST to
// the operator's own token URL is still the only thing the token host takes.
func TestPlaceholderBaseURLRefusesEveryAPIRequest(t *testing.T) {
	pins := newNerdioPins("https://nmm.example.com", guardEntraToken)
	for _, c := range []struct{ method, url string }{
		{http.MethodGet, "https://nmm.example.com/rest-api/v1/accounts"},
		{http.MethodGet, "https://NMM.example.com:443/rest-api/v1-beta/accounts/101/devices"},
		{http.MethodPost, "https://nmm.example.com/rest-api/v1/scripted-actions/7/execution"},
	} {
		req, _ := http.NewRequest(c.method, c.url, nil)
		req.Header.Set("Authorization", "Bearer minted")
		err := pins.check(req)
		if !errors.Is(err, ErrOriginChanged) || !strings.Contains(err.Error(), "NERDIO_BASE_URL") {
			t.Errorf("%s %s with the placeholder base URL: want a refusal naming NERDIO_BASE_URL, got %v", c.method, c.url, err)
		}
	}
	tok, _ := http.NewRequest(http.MethodPost, guardEntraToken, nil)
	if err := pins.check(tok); err != nil {
		t.Errorf("the token POST to the operator's own token URL was refused: %v", err)
	}

	// The same on a live run: no NERDIO_BASE_URL (the shipped placeholder),
	// the operator's Entra tenant and client pair set. The command and doctor
	// refuse naming NERDIO_BASE_URL, and the NMM API request never leaves
	// (the pin refuses it before any dial: the placeholder name is never even
	// resolved). The network fence keeps a broken pin from reaching the real
	// placeholder host.
	fence := fenceNetwork(t)
	m := newFakeInstance(t, "placeholderClientId-2e7a", "placeholderSecret-6c1d", "placeholderMinted-9b3f", func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("the fake NMM instance was reached: %s %s", r.Method, r.URL.Path)
	})
	credRulesHome(t)
	m.useAs(t)
	t.Setenv("NERDIO_BASE_URL", "")
	t.Setenv("NERDIO_NO_CONFIG_WRITE", "1")
	for _, args := range [][]string{{"accounts", "--json"}, {"doctor", "--json"}} {
		var out bytes.Buffer
		cmd := newRootCmd(&rootFlags{})
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs(args)
		_ = cmd.Execute()
		if !strings.Contains(out.String(), "NERDIO_BASE_URL") {
			t.Errorf("%v with the placeholder base URL: the output does not name NERDIO_BASE_URL:\n%s", args, out.String())
		}
		if strings.Contains(out.String(), "placeholderMinted-9b3f") || strings.Contains(out.String(), "placeholderSecret-6c1d") {
			t.Errorf("%v printed a credential:\n%s", args, out.String())
		}
	}
	if m.other.Load() != 0 || m.bad.Load() != 0 {
		t.Errorf("the token host received %d request(s) that were not the token POST and %d refused mint(s)", m.other.Load(), m.bad.Load())
	}
	if dialed, escaped := fence.result(); len(escaped) != 0 || len(dialed) == 0 {
		t.Errorf("want only loopback dials through the fence (the mint at the fake tenant), got dialed %v, escaped %v", dialed, escaped)
	}
}

// The same rule on a live client: with NERDIO_TOKEN_URL on another origin,
// the mint posts the pair with HTTP Basic to that exact path and the API
// request carries the minted bearer to the API origin; an API request aimed
// at the token host is refused before it is sent; and a redirect hop must stay
// on the origin of the request it follows, even to the other pinned origin.
func TestGuardSendsOnlyTheTokenPostToTheTokenHost(t *testing.T) {
	const id, secret, minted = "guardClientId-4c1e", "guardClientSecret-8a3d6f", "guardMinted-2b7e9d"
	var apiAuth atomic.Value
	var m *fakeInstance
	m = newFakeInstance(t, id, secret, minted, func(w http.ResponseWriter, r *http.Request) {
		apiAuth.Store(r.Header.Get("Authorization"))
		if r.URL.Path == "/rest-api/v1/redirect-to-token-host" {
			http.Redirect(w, r, m.tokenURL, http.StatusTemporaryRedirect)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `[]`)
	})
	credRulesHome(t)
	m.useAs(t)
	t.Setenv("NERDIO_NO_CONFIG_WRITE", "1")
	flags := rootFlags{timeout: 10 * time.Second}
	c, err := flags.newClient()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Get(context.Background(), "/rest-api/v1/accounts", nil); err != nil {
		t.Fatalf("read through the token host's mint: %v", err)
	}
	if m.mints.Load() != 1 || apiAuth.Load() != "Bearer "+minted {
		t.Fatalf("want one mint at the token host and the minted bearer on the API origin; mints %d (refused %d), API saw %v", m.mints.Load(), m.bad.Load(), apiAuth.Load())
	}
	for _, probe := range []struct{ method, url, auth string }{
		{http.MethodGet, m.entra.URL + "/rest-api/v1/accounts", "Bearer " + minted},
		{http.MethodPost, m.tokenURL, "Bearer " + minted},
		{http.MethodGet, m.tokenURL, ""},
	} {
		req, _ := http.NewRequest(probe.method, probe.url, nil)
		if probe.auth != "" {
			req.Header.Set("Authorization", probe.auth)
		}
		resp, err := c.HTTPClient.Do(req)
		if resp != nil {
			_ = resp.Body.Close()
		}
		if err == nil || !strings.Contains(err.Error(), "leave the configured Nerdio host") {
			t.Errorf("%s %s (%q) on the token host: want the pin refusal, got %v", probe.method, probe.url, probe.auth, err)
		}
	}
	if _, err := c.Get(context.Background(), "/rest-api/v1/redirect-to-token-host", nil); err == nil || !strings.Contains(err.Error(), "leave the configured Nerdio host") {
		t.Errorf("an API read redirected to the token host: want the pin refusal, got %v", err)
	}
	// A 307 on an API write resends its method and body; to the token URL
	// the hop is a POST of the exact token path with no Authorization (the
	// transport drops it across hosts), which the token pin alone would let
	// through, carrying the API body to the token host. The hop must stay on
	// the origin of the request it follows.
	if _, _, err := c.Post(context.Background(), "/rest-api/v1/redirect-to-token-host", map[string]any{"name": "API body for the API host only"}); err == nil || !strings.Contains(err.Error(), "leave the configured Nerdio host") {
		t.Errorf("an API write redirected to the token host: want the pin refusal, got %v", err)
	}
	if n := m.other.Load() + m.bad.Load(); n != 0 || m.mints.Load() != 1 {
		t.Fatalf("the token host received %d request(s) that were not the one accepted token POST (mints %d)", n, m.mints.Load())
	}
}

// auth login mints at NERDIO_TOKEN_URL (the operator's Entra tenant, another
// host than the NMM instance): HTTP Basic, the documented form with exactly
// NERDIO_OAUTH_SCOPE, and the token lands in the 0600 credential store, never
// config.toml. The press defaulted the token URL to a placeholder tenant
// (nerdio handfix token-url-from-operator-only).
// TestAuthLoginMintsOnlyAtItsTokenURL (nerdioblackbox) proves on the built
// binary, through a recording proxy, that nothing leaves for another host.
func TestAuthLoginMintsAtTheConfiguredTokenURL(t *testing.T) {
	const id, secret, minted = "loginClientId-7e2c", "loginClientSecret-3f9a1b", "loginMinted-5d8c2e"
	var apiHits atomic.Int32
	m := newFakeInstance(t, id, secret, minted, func(w http.ResponseWriter, r *http.Request) {
		apiHits.Add(1)
		w.WriteHeader(http.StatusNotFound)
	})
	credRulesHome(t)
	home := os.Getenv("NERDIO_HOME")
	m.useAs(t)
	var out bytes.Buffer
	cmd := newRootCmd(&rootFlags{})
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"auth", "login"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("auth login against the configured Entra tenant: %v\n%s", err, out.String())
	}
	if m.mints.Load() != 1 || m.bad.Load() != 0 || m.other.Load() != 0 {
		t.Fatalf("want one accepted mint at %s, got %d (refused %d, other %d; last %v)", m.tokenURL, m.mints.Load(), m.bad.Load(), m.other.Load(), m.lastMintForm.Load())
	}
	if apiHits.Load() != 0 {
		t.Errorf("auth login sent %d request(s) to the NMM instance", apiHits.Load())
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

// token-url-from-operator-only and oauth-scope-required on auth login: with
// NERDIO_TOKEN_URL unset or still naming the placeholder tenant
// (YOUR_TENANT_ID; once on the real Entra host, once on the fake tenant's
// host so a regression is counted there), or NERDIO_OAUTH_SCOPE unset, auth
// login sends nothing to either fake or any other host (the network fence
// records and refuses every non-loopback dial: the press's placeholder
// fallback was the real login.microsoftonline.com), its error names the
// variable to set, and nothing is saved.
func TestAuthLoginWithoutTokenURLOrScopeSendsNothing(t *testing.T) {
	for _, c := range []struct {
		name, variable string
		tokenURL       func(m *fakeInstance) string
		scope          string
	}{
		{"NERDIO_TOKEN_URL unset", "NERDIO_TOKEN_URL", func(*fakeInstance) string { return "" }, nerdioTestScope},
		{"NERDIO_TOKEN_URL the placeholder tenant", "NERDIO_TOKEN_URL", func(m *fakeInstance) string {
			return "https://login.microsoftonline.com/YOUR_TENANT_ID/oauth2/v2.0/token"
		}, nerdioTestScope},
		{"NERDIO_TOKEN_URL the placeholder tenant on the fake host", "NERDIO_TOKEN_URL", func(m *fakeInstance) string {
			return m.entra.URL + "/YOUR_TENANT_ID/oauth2/v2.0/token"
		}, nerdioTestScope},
		{"NERDIO_OAUTH_SCOPE unset", "NERDIO_OAUTH_SCOPE", func(m *fakeInstance) string { return m.tokenURL }, ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			fence := fenceNetwork(t)
			var apiHits atomic.Int32
			m := newFakeInstance(t, "noUrlClientId-5a1c", "noUrlClientSecret-2e8b", "noUrlMinted-7d3f", func(w http.ResponseWriter, r *http.Request) {
				apiHits.Add(1)
			})
			credRulesHome(t)
			home := os.Getenv("NERDIO_HOME")
			m.useAs(t)
			t.Setenv("NERDIO_TOKEN_URL", c.tokenURL(m))
			t.Setenv("NERDIO_OAUTH_SCOPE", c.scope)
			var out bytes.Buffer
			cmd := newRootCmd(&rootFlags{})
			cmd.SetOut(&out)
			cmd.SetErr(&out)
			cmd.SetArgs([]string{"auth", "login"})
			err := cmd.Execute()
			if err == nil || !strings.Contains(err.Error()+out.String(), c.variable) {
				t.Errorf("auth login: want an error naming %s, got %v\n%s", c.variable, err, out.String())
			}
			if n := m.mints.Load() + m.bad.Load() + m.other.Load() + apiHits.Load(); n != 0 {
				t.Errorf("auth login sent %d request(s) (token host: %d accepted, %d refused, %d other; NMM instance %d)", n, m.mints.Load(), m.bad.Load(), m.other.Load(), apiHits.Load())
			}
			if dialed, _ := fence.result(); len(dialed) != 0 {
				t.Errorf("auth login dialed %v", dialed)
			}
			_ = filepath.WalkDir(home, func(path string, d os.DirEntry, err error) error {
				if err == nil && !d.IsDir() {
					if b, _ := os.ReadFile(path); bytes.Contains(b, []byte("noUrlClientSecret-2e8b")) {
						t.Errorf("a refused login saved the client secret in %s", path)
					}
				}
				return nil
			})
		})
	}
}

// Redirect hops must stay on the origin of the request they follow, even when
// the target is the other pinned origin: a 307 on an API write to the token
// URL is a bodied POST of the exact token path with its Authorization dropped,
// which the token pin alone accepts. The check runs on the installed redirect
// policy itself, with an IP-literal token host so no name is resolved.
func TestRedirectHopStaysOnItsOrigin(t *testing.T) {
	const base, tokenURL = "https://nmm.contoso.com", "https://203.0.113.10/7c1e9a3f/oauth2/v2.0/token"
	hc := &http.Client{}
	installPins(hc, newNerdioPins(base, tokenURL), false)
	hop := func(fromMethod, from, to string) error {
		via, _ := http.NewRequest(fromMethod, from, nil)
		req, _ := http.NewRequest(fromMethod, to, nil)
		return hc.CheckRedirect(req, []*http.Request{via})
	}
	if err := hop(http.MethodPost, base+"/rest-api/v1/accounts/101/host-pool", tokenURL); !errors.Is(err, ErrOriginChanged) {
		t.Errorf("an API write redirected to the token URL: want ErrOriginChanged, got %v", err)
	}
	if err := hop(http.MethodPost, tokenURL, base+"/rest-api/v1/accounts"); !errors.Is(err, ErrOriginChanged) {
		t.Errorf("the token request redirected to the API origin: want ErrOriginChanged, got %v", err)
	}
	if err := hop(http.MethodGet, base+"/rest-api/v1/accounts", "https://evil.example/rest-api/v1/accounts"); !errors.Is(err, ErrOriginChanged) {
		t.Errorf("an API read redirected to another host: want ErrOriginChanged, got %v", err)
	}
	if err := hop(http.MethodGet, base+"/rest-api/v1/accounts", base+"/rest-api/v1/accounts?page=1"); err != nil {
		t.Errorf("a same-origin hop must be followed: %v", err)
	}
}
