// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package config

// kaseya-bms handfixes token-minted-per-client, minted-token-bound-to-login-and-host,
// mint-pinned-to-api-host, token-identity-follows-the-configured-credential and
// store-scope-stable-across-mints, at the unit level (the black-box suite in
// internal/kbmsblackbox proves them on the built binaries).

import (
	"context"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"kaseya-bms-pp-cli/internal/cliutil"
)

const (
	tUser     = "unit.user@example.com"
	tTenant   = "unittenant"
	tPassword = "unitLoginPassword-7c2e9a1b"
	tAccess   = "unitMintedAccess-3f8b1d6e"
	tRefresh  = "unitMintedRefresh-9a4c2e7f"
)

func setUnitLogin(t *testing.T) {
	t.Helper()
	t.Setenv(KbmsEnvUsername, tUser)
	t.Setenv(KbmsEnvLoginVar, tPassword)
	t.Setenv(KbmsEnvTenant, tTenant)
}

// fakeLogin answers the BMS login and records every request it saw.
func fakeLogin(t *testing.T, mints *int32) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/security/authenticate" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		atomic.AddInt32(mints, 1)
		mt, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if r.Method != http.MethodPost || err != nil || mt != "multipart/form-data" || r.Header.Get("Authorization") != "" {
			t.Errorf("login request: method %s, content type %q, Authorization %q", r.Method, r.Header.Get("Content-Type"), r.Header.Get("Authorization"))
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		form := map[string]string{}
		mr := multipart.NewReader(r.Body, params["boundary"])
		for {
			p, err := mr.NextPart()
			if err != nil {
				break
			}
			b, _ := io.ReadAll(p)
			form[p.FormName()] = string(b)
		}
		want := map[string]string{"UserName": tUser, "Password": tPassword, "Tenant": tTenant, "GrantType": "password"}
		for k, v := range want {
			if form[k] != v {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = io.WriteString(w, `{"Success":false,"Error":{"Code":978001,"Message":"Security Error","Details":"bad `+k+` `+form[k]+`"},"Result":null}`)
				return
			}
		}
		_, _ = io.WriteString(w, `{"Success":true,"Error":null,"Result":{"AccessToken":"`+tAccess+`","RefreshToken":"`+tRefresh+`","AccessTokenExpireOn":"2099-01-01T00:00:00Z"}}`)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestMintSendsTheBMSLoginForm(t *testing.T) {
	var mints int32
	srv := fakeLogin(t, &mints)
	tok, err := MintKbmsToken(context.Background(), nil, srv.URL, KbmsLogin{Username: tUser, Password: tPassword, Tenant: tTenant})
	if err != nil {
		t.Fatal(err)
	}
	if tok.Access != tAccess || tok.Refresh != tRefresh || tok.Expiry.Year() != 2099 {
		t.Fatalf("minted %+v", tok)
	}
	if !cliutil.Held(tAccess) || !cliutil.Held(tRefresh) || !cliutil.Held(tPassword) {
		t.Fatal("the password and the minted tokens must be held before the mint returns")
	}
}

func TestFailedLoginRepeatsNoHeldValue(t *testing.T) {
	var mints int32
	srv := fakeLogin(t, &mints)
	const wrong = "unitWrongPassword-5d1a8c3e"
	_, err := MintKbmsToken(context.Background(), nil, srv.URL, KbmsLogin{Username: tUser, Password: wrong, Tenant: tTenant})
	if err == nil {
		t.Fatal("a wrong password minted a token")
	}
	if strings.Contains(err.Error(), wrong) {
		t.Fatalf("the login error repeats the password: %v", err)
	}
	if !strings.Contains(err.Error(), "HTTP 401") {
		t.Fatalf("the login error does not name the refusal: %v", err)
	}
}

func TestMintNeverFollowsARedirectNorLeavesTheHost(t *testing.T) {
	var other int32
	elsewhere := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { atomic.AddInt32(&other, 1) }))
	t.Cleanup(elsewhere.Close)
	same := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/v2/security/other", http.StatusTemporaryRedirect)
	}))
	t.Cleanup(same.Close)
	off := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, elsewhere.URL+r.URL.Path, http.StatusTemporaryRedirect)
	}))
	t.Cleanup(off.Close)
	l := KbmsLogin{Username: tUser, Password: tPassword, Tenant: tTenant}
	for name, base := range map[string]string{"same-host redirect": same.URL, "off-host redirect": off.URL} {
		if _, err := MintKbmsToken(context.Background(), nil, base, l); err == nil || !strings.Contains(err.Error(), "redirect") {
			t.Errorf("%s: want the redirect refused, got %v", name, err)
		}
		// Through a caller's client that would follow redirects, the mint
		// still follows none.
		if _, err := MintKbmsToken(context.Background(), &http.Client{Timeout: 5 * time.Second}, base, l); err == nil {
			t.Errorf("%s: a redirect-following client minted through the redirect", name)
		}
	}
	if n := atomic.LoadInt32(&other); n != 0 {
		t.Fatalf("the login reached another host %d time(s)", n)
	}
}

func TestMintRefusesPlaintextOffLoopback(t *testing.T) {
	l := KbmsLogin{Username: tUser, Password: tPassword, Tenant: tTenant}
	_, err := MintKbmsToken(context.Background(), nil, "http://bms.example.com", l)
	if err == nil || !strings.Contains(err.Error(), "plaintext") {
		t.Fatalf("want plaintext http refused off loopback, got %v", err)
	}
	for _, base := range []string{"https://user:pw@api.bms.kaseya.com", "ftp://api.bms.kaseya.com", "https://api.bms.kasеya.com"} {
		if _, err := MintKbmsToken(context.Background(), nil, base, l); err == nil {
			t.Errorf("%s: want no mint", base)
		}
	}
}

func TestBindingNamesTheHostAndTheLogin(t *testing.T) {
	l := KbmsLogin{Username: tUser, Tenant: tTenant}
	a := KbmsTokenBinding("https://api.bms.kaseya.com", l)
	if len(a) != 32 {
		t.Fatalf("binding %q: want 32 hex digits", a)
	}
	for name, b := range map[string]string{
		"another host":     KbmsTokenBinding("https://api.bmsemea.kaseya.com", l),
		"another user":     KbmsTokenBinding("https://api.bms.kaseya.com", KbmsLogin{Username: "other@example.com", Tenant: tTenant}),
		"another tenant":   KbmsTokenBinding("https://api.bms.kaseya.com", KbmsLogin{Username: tUser, Tenant: "othertenant"}),
		"another port":     KbmsTokenBinding("https://api.bms.kaseya.com:8443", l),
		"plaintext scheme": KbmsTokenBinding("http://api.bms.kaseya.com", l),
	} {
		if b == a || b == "" {
			t.Errorf("%s: binding %q must differ from %q", name, b, a)
		}
	}
	if KbmsTokenBinding("https://API.BMS.KASEYA.COM:443/", l) != a {
		t.Error("the same host spelled differently must bind the same")
	}
	if KbmsTokenBinding("https://api.bms.kaseya.com", KbmsLogin{Tenant: tTenant}) != "" {
		t.Error("no username: no binding")
	}
	for _, s := range []string{tUser, tTenant, "kaseya", "bms"} {
		if strings.Contains(a, s) {
			t.Errorf("the binding %q carries %q", a, s)
		}
	}
}

func boundConfig(base, binding string) *Config {
	return &Config{BaseURL: base, AccessToken: "storedAccess-1a2b3c", RefreshToken: "storedRefresh-4d5e6f",
		TokenExpiry: time.Now().Add(time.Hour), ClientID: binding, AuthHeaderVal: "Bearer staleHeader-7a8b9c",
		Headers: map[string]string{"Authorization": "Bearer headerToken-1c2d3e", "X-Trace": "keep-me"}, envOverrides: map[string]bool{}}
}

func TestEnvironmentTokenIsTheOnlyBearer(t *testing.T) {
	setUnitLogin(t)
	c := boundConfig("https://api.bms.kaseya.com", KbmsTokenBinding("https://api.bms.kaseya.com", KbmsEnvLogin()))
	c.KaseyaBmsToken = "envBearer-9f8e7d"
	c.envOverrides["KaseyaBmsToken"] = true
	dropKbmsTokenOutsideItsBinding(c)
	if c.AccessToken != "" || c.AuthHeaderVal != "" || c.ClientID != "" {
		t.Fatalf("an environment token kept a stored credential: %+v", c)
	}
	if _, ok := c.Headers["Authorization"]; ok {
		t.Fatal("a [headers] Authorization survived beside the environment token")
	}
	if c.Headers["X-Trace"] != "keep-me" {
		t.Fatal("a header that carries no credential was dropped")
	}
	if got := c.AuthHeader(); got != "Bearer envBearer-9f8e7d" {
		t.Fatalf("AuthHeader = %q, want the environment token", got)
	}
	if kbmsStoreScope(c) != "" {
		t.Fatal("an environment token must keep the token-hash store scope, never the login's store")
	}
}

func TestConfiguredLoginAllowsOnlyItsOwnToken(t *testing.T) {
	setUnitLogin(t)
	base := "https://api.bms.kaseya.com"
	own := KbmsTokenBinding(base, KbmsEnvLogin())
	other := KbmsTokenBinding(base, KbmsLogin{Username: "someone.else@example.com", Tenant: tTenant})
	c := boundConfig(base, other)
	c.KaseyaBmsToken = "fileBmsToken-2b3c4d" // from config.toml, not the environment
	dropKbmsTokenOutsideItsBinding(c)
	if c.AccessToken != "" || c.KaseyaBmsToken != "" || c.AuthHeaderVal != "" {
		t.Fatalf("a token bound to another login was kept: %+v", c)
	}
	if _, ok := c.Headers["Authorization"]; ok {
		t.Fatal("a [headers] Authorization survived beside the configured login")
	}
	c = boundConfig(base, own)
	dropKbmsTokenOutsideItsBinding(c)
	if c.AccessToken != "storedAccess-1a2b3c" {
		t.Fatal("the login's own bound token was dropped")
	}
	if kbmsStoreScope(c) != "kaseya-bms-store|"+own {
		t.Fatalf("store scope = %q, want the login's binding", kbmsStoreScope(c))
	}
}

func TestStoredTokenNeverReachesAnotherHost(t *testing.T) {
	// No login configured: a stored minted token bound to host A is dropped on host B.
	hostA, hostB := "https://api.bms.kaseya.com", "https://api.bmsemea.kaseya.com"
	c := boundConfig(hostB, KbmsTokenBinding(hostA, KbmsLogin{Username: tUser, Tenant: tTenant}))
	c.Headers = nil
	c.AuthHeaderVal = ""
	dropKbmsTokenOutsideItsBinding(c)
	if c.AccessToken != "" {
		t.Fatal("a token minted on another host was kept")
	}
	// A set-token token (no binding) stays the operator's choice.
	c = boundConfig(hostB, "")
	c.Headers, c.AuthHeaderVal = nil, ""
	dropKbmsTokenOutsideItsBinding(c)
	if c.AccessToken == "" {
		t.Fatal("an unbound auth set-token token was dropped with no login configured")
	}
}

func TestEnsureMintsOncePerBindingAndReusesTheLiveToken(t *testing.T) {
	setUnitLogin(t)
	var mints int32
	srv := fakeLogin(t, &mints)
	kbmsMintMu.Lock()
	kbmsProcToken = map[string]kbmsProcessToken{}
	kbmsMintMu.Unlock()
	t.Setenv("KASEYA_BMS_HOME", t.TempDir())
	for i := 0; i < 3; i++ {
		c := &Config{BaseURL: srv.URL, envOverrides: map[string]bool{}}
		if err := EnsureKbmsTokenWith(context.Background(), nil, c); err != nil {
			t.Fatal(err)
		}
		if c.AccessToken != tAccess || c.ClientID != KbmsTokenBinding(srv.URL, KbmsEnvLogin()) {
			t.Fatalf("call %d: token %q binding %q", i, c.AccessToken, c.ClientID)
		}
	}
	if n := atomic.LoadInt32(&mints); n != 1 {
		t.Fatalf("%d mints for one binding in one process, want 1", n)
	}
	// A live stored token bound to the login is reused without a mint; an
	// expired one is replaced.
	kbmsMintMu.Lock()
	kbmsProcToken = map[string]kbmsProcessToken{}
	kbmsMintMu.Unlock()
	live := &Config{BaseURL: srv.URL, AccessToken: "storedLive-5e6f7a", ClientID: KbmsTokenBinding(srv.URL, KbmsEnvLogin()),
		TokenExpiry: time.Now().Add(time.Hour), envOverrides: map[string]bool{}}
	if err := EnsureKbmsTokenWith(context.Background(), nil, live); err != nil || live.AccessToken != "storedLive-5e6f7a" {
		t.Fatalf("a live bound token was not reused: %v %q", err, live.AccessToken)
	}
	expired := &Config{BaseURL: srv.URL, AccessToken: "storedExpired-8b9c0d", ClientID: KbmsTokenBinding(srv.URL, KbmsEnvLogin()),
		TokenExpiry: time.Now().Add(-time.Minute), envOverrides: map[string]bool{}}
	if err := EnsureKbmsTokenWith(context.Background(), nil, expired); err != nil || expired.AccessToken != tAccess {
		t.Fatalf("an expired bound token was not re-minted: %v %q", err, expired.AccessToken)
	}
	if n := atomic.LoadInt32(&mints); n != 2 {
		t.Fatalf("%d mints, want 2 (the reuse sent none)", n)
	}
}
