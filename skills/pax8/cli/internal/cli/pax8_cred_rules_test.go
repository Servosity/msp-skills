// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"pax8-pp-cli/internal/client"
	"pax8-pp-cli/internal/cliutil"
	"pax8-pp-cli/internal/cliutil/testenv"
	"pax8-pp-cli/internal/config"
	"pax8-pp-cli/internal/credkit"
)

// api-origin-pinned: the client ID and secret reach only the token URL, the
// minted bearer token reaches only the API host, a redirect to another host is
// refused before it is followed (from the API host and from the token host),
// and the configured hosts still answer.
func TestOriginPinKeepsEachCredentialOnItsHost(t *testing.T) {
	const id, secret, minted = "pinUnitClientId-5b2d8f1a", "pinUnitSecret-4c1e7a9b3d", "pinUnitMinted-7e3a9c1f5b"
	var otherHits, tokenHits atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		otherHits.Add(1)
		_, _ = w.Write([]byte(`{"content":[]}`))
	}))
	t.Cleanup(other.Close)
	tok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokenHits.Add(1)
		_ = r.ParseForm()
		if r.PostForm.Get("client_secret") != secret || r.PostForm.Get("client_id") != id {
			t.Errorf("the token host got form %v", r.PostForm)
		}
		_, _ = w.Write([]byte(`{"access_token":"` + minted + `","expires_in":3600}`))
	}))
	t.Cleanup(tok.Close)
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if strings.Contains(string(body)+r.URL.String()+r.Header.Get("Authorization"), secret) {
			t.Errorf("the API host received the client secret")
		}
		if r.Header.Get("Authorization") != "Bearer "+minted {
			t.Errorf("the API host got Authorization %q", r.Header.Get("Authorization"))
		}
		if strings.HasSuffix(r.URL.Path, "/redirect") {
			http.Redirect(w, r, other.URL+"/v1/companies", http.StatusTemporaryRedirect)
			return
		}
		_, _ = w.Write([]byte(`{"content":[{"id":"c1"}],"page":{"size":10,"totalElements":1,"totalPages":1,"number":0}}`))
	}))
	t.Cleanup(api.Close)
	testenv.Isolate(t)
	t.Setenv("PAX8_BASE_URL", api.URL+"/v1")
	t.Setenv("PAX8_TOKEN_URL", tok.URL+"/oauth/token")
	t.Setenv("PAX8_CLIENT_ID", id)
	t.Setenv("PAX8_CLIENT_SECRET", secret)
	t.Setenv("PAX8_NO_CONFIG_WRITE", "1")
	flags := rootFlags{timeout: 10 * time.Second}
	c, err := flags.newClient()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Get(context.Background(), "/companies", nil); err != nil {
		t.Fatalf("configured hosts: %v", err)
	}
	_, err = c.Get(context.Background(), "/redirect", nil)
	if err == nil || !strings.Contains(err.Error(), "leave the configured Pax8 host") {
		t.Fatalf("redirect: want the pin refusal, got %v", err)
	}
	if otherHits.Load() != 0 {
		t.Fatalf("the other host was contacted %d time(s)", otherHits.Load())
	}
	if tokenHits.Load() == 0 {
		t.Fatal("the token host was never asked")
	}
}

// A token endpoint that redirects the mint elsewhere never gets the client
// secret replayed to the other host.
func TestOriginPinRefusesTokenRedirect(t *testing.T) {
	var otherHits atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { otherHits.Add(1) }))
	t.Cleanup(other.Close)
	tok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+"/oauth/token", http.StatusTemporaryRedirect)
	}))
	t.Cleanup(tok.Close)
	testenv.Isolate(t)
	t.Setenv("PAX8_BASE_URL", "https://api.pax8.invalid/v1")
	t.Setenv("PAX8_TOKEN_URL", tok.URL+"/oauth/token")
	t.Setenv("PAX8_CLIENT_ID", "redirUnitClientId-8c4e2a6b")
	t.Setenv("PAX8_CLIENT_SECRET", "redirUnitSecret-1d9f3b7e5a")
	t.Setenv("PAX8_NO_CONFIG_WRITE", "1")
	flags := rootFlags{timeout: 10 * time.Second}
	c, err := flags.newClient()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Get(context.Background(), "/companies", nil); err == nil {
		t.Fatal("a redirected mint succeeded")
	}
	if otherHits.Load() != 0 {
		t.Fatalf("the redirect target received the token request %d time(s)", otherHits.Load())
	}
}

// What the token host may receive: only the token POST, on the configured
// path, with no Authorization header. Everything else off the API origin is
// refused, including a scheme downgrade and a non-ASCII host.
func TestPinsAllowOnlyTheTokenRequestOnTheTokenHost(t *testing.T) {
	pins := newPax8Pins("https://api.pax8.com/v1", "")
	for _, tc := range []struct {
		method, raw, auth string
		ok                bool
	}{
		{"GET", "https://api.pax8.com/v1/companies", "Bearer x", true},
		{"PATCH", "https://API.PAX8.COM:443/v1/companies/1", "Bearer x", true},
		{"POST", "https://token-manager.pax8.com/oauth/token", "", true},
		{"POST", "https://token-manager.pax8.com/oauth/token", "Bearer x", false},
		{"GET", "https://token-manager.pax8.com/oauth/token", "", false},
		{"POST", "https://token-manager.pax8.com/oauth/other", "", false},
		{"GET", "http://api.pax8.com/v1/companies", "Bearer x", false},
		{"GET", "https://api.pax8.com:8443/v1/companies", "Bearer x", false},
		{"GET", "https://evil.example.com/v1/companies", "Bearer x", false},
		{"GET", "https://api.pаx8.com/v1/companies", "Bearer x", false},
	} {
		u, err := url.Parse(tc.raw)
		if err != nil {
			t.Fatal(err)
		}
		req := &http.Request{Method: tc.method, URL: u, Header: http.Header{}}
		if tc.auth != "" {
			req.Header.Set("Authorization", tc.auth)
		}
		if err := pins.check(req); (err == nil) != tc.ok {
			t.Errorf("%s %s (auth %q): allowed=%v, want %v (%v)", tc.method, tc.raw, tc.auth, err == nil, tc.ok, err)
		}
	}
	// The auth login token client reaches the token URL alone.
	login := newPax8Pins("", "https://token-manager.pax8.com/oauth/token")
	if err := login.check(&http.Request{Method: "GET", URL: &url.URL{Scheme: "https", Host: "api.pax8.com", Path: "/v1/companies"}, Header: http.Header{}}); err == nil {
		t.Error("the login token client may reach the API host")
	}
}

// The pin is the outermost transport, the response cache is off, a second
// hook run does not wrap the chain again, and under --dry-run nothing is sent.
func TestGuardWrapsTheTransport(t *testing.T) {
	c := &client.Client{BaseURL: "https://api.pax8.com/v1", HTTPClient: &http.Client{}}
	guardClient(c)
	pin, ok := c.HTTPClient.Transport.(originPinTripper)
	if !ok {
		t.Fatalf("the origin pin is not outermost: %T", c.HTTPClient.Transport)
	}
	if pin.next == nil {
		t.Fatal("the pin sends through no transport")
	}
	if !c.NoCache {
		t.Fatal("the response cache is on")
	}
	guardClient(c)
	if again, ok := c.HTTPClient.Transport.(originPinTripper); !ok || again.next != pin.next {
		t.Fatal("a second guard run re-wrapped the transport")
	}
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1) }))
	t.Cleanup(srv.Close)
	dry := &client.Client{BaseURL: srv.URL, HTTPClient: &http.Client{}, DryRun: true}
	guardClient(dry)
	req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/subscriptions/1", nil)
	if _, err := dry.HTTPClient.Do(req); !errors.Is(err, ErrDryRunSend) {
		t.Fatalf("dry run: want ErrDryRunSend, got %v", err)
	}
	if hits.Load() != 0 {
		t.Fatal("a --dry-run client sent a request")
	}
}

func TestNormalizedOriginRefusesNonASCIIAndFillsPorts(t *testing.T) {
	for raw, want := range map[string]string{
		"https://API.Pax8.com/v1":        "https://api.pax8.com:443",
		"https://api.pax8.com:8443/x":    "https://api.pax8.com:8443",
		"http://127.0.0.1/x":             "http://127.0.0.1:80",
		"https://api.pаx8.com/v1/x":      "",
		"https://token-manager.pax8.com": "https://token-manager.pax8.com:443",
	} {
		u, _ := url.Parse(raw)
		if got := normalizedOrigin(u); got != want {
			t.Errorf("%s: got %q, want %q", raw, got, want)
		}
	}
}

// api-origin-pinned for 'auth login': the client secret is posted to the
// configured token URL only; a redirect is refused and never followed, a
// token URL other than the configured one is refused, the minted token is
// held, and under --dry-run nothing is sent.
func TestAuthLoginMintIsPinnedAndHeld(t *testing.T) {
	var otherHits atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { otherHits.Add(1) }))
	t.Cleanup(other.Close)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redirect/oauth/token" {
			http.Redirect(w, r, other.URL+"/steal", http.StatusTemporaryRedirect)
			return
		}
		_, _ = w.Write([]byte(`{"access_token":"mintedLoginTokenUnit-91c2d3e4","expires_in":3599,"token_type":"Bearer"}`))
	}))
	t.Cleanup(srv.Close)
	t.Setenv("PAX8_AUDIENCE", "")
	var held []string
	hold := cliutil.Hold
	cliutil.Hold = func(_ credkit.Source, vs ...string) { held = append(held, vs...) }
	t.Cleanup(func() { cliutil.Hold = hold })
	const secret = "mintSecretUnit-4b8d2f6a"
	login := func(configured string, dryRun bool) *http.Client {
		hc := &http.Client{}
		guardTokenClient(hc, &config.Config{TokenURL: configured}, dryRun)
		return hc
	}
	if _, err := mintClientCredentialsToken(login(srv.URL+"/redirect/oauth/token", false), srv.URL+"/redirect/oauth/token", "app-id", secret); err == nil {
		t.Fatal("a redirected login mint succeeded")
	}
	if otherHits.Load() != 0 {
		t.Fatalf("the redirect target was contacted %d time(s)", otherHits.Load())
	}
	if _, err := mintClientCredentialsToken(login(srv.URL+"/oauth/token", false), other.URL+"/oauth/token", "app-id", secret); err == nil || otherHits.Load() != 0 {
		t.Fatalf("a token URL other than the configured one was dialled (%v, %d hits)", err, otherHits.Load())
	}
	if _, err := mintClientCredentialsToken(login(srv.URL+"/oauth/token", true), srv.URL+"/oauth/token", "app-id", secret); !errors.Is(err, ErrDryRunSend) {
		t.Fatalf("dry run: want ErrDryRunSend, got %v", err)
	}
	tok, err := mintClientCredentialsToken(login(srv.URL+"/oauth/token", false), srv.URL+"/oauth/token", "app-id", secret)
	if err != nil || tok.AccessToken != "mintedLoginTokenUnit-91c2d3e4" {
		t.Fatalf("mint: got %+v, %v", tok, err)
	}
	if !slices.Contains(held, "mintedLoginTokenUnit-91c2d3e4") {
		t.Errorf("the minted login token is not held with the kit (held %d values)", len(held))
	}
}

// A redirect hop stays on the origin of the request it follows, even between
// the two pinned hosts: an API 307 naming the token path would otherwise replay
// a write body to the token host.
func TestRedirectStaysOnItsOwnOrigin(t *testing.T) {
	var tokenHits atomic.Int32
	tok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { tokenHits.Add(1) }))
	t.Cleanup(tok.Close)
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, tok.URL+"/oauth/token", http.StatusTemporaryRedirect)
	}))
	t.Cleanup(api.Close)
	hc := &http.Client{}
	installPins(hc, newPax8Pins(api.URL+"/v1", tok.URL+"/oauth/token"), false)
	req, _ := http.NewRequest(http.MethodPost, api.URL+"/v1/orders", strings.NewReader(`{"companyId":"c1"}`))
	if _, err := hc.Do(req); err == nil || !strings.Contains(err.Error(), "leave the configured Pax8 host") {
		t.Fatalf("want the pin refusal, got %v", err)
	}
	if tokenHits.Load() != 0 {
		t.Fatalf("the token host received the redirected write %d time(s)", tokenHits.Load())
	}
}
