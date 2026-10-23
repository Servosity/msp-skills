// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"net/http"
	"testing"
	"time"

	"sherweb-pp-cli/internal/config"
)

// Hand-owned (handfixes.json: redirect-strips-every-credential). These drive
// the client's real CheckRedirect with the headers Go copies onto the next hop.

// redirectTestCredentials is every header this connector can fill with a
// credential, named here rather than read from redirectCredentialHeaders so the
// test cannot agree with a wrong list.
var redirectTestCredentials = map[string]string{
	"Authorization":             "Bearer primary-credential",
	"Proxy-Authorization":       "Basic cHJveHk6c2VjcmV0",
	"Cookie":                    "session=cookie-credential",
	"X-Config-Header":           "config-credential",
	"Ocp-Apim-Subscription-Key": "connector-credential",
}

func newRedirectTestClient(t *testing.T) *Client {
	t.Helper()
	for _, k := range []string{"HOME", "USERPROFILE", "XDG_CACHE_HOME"} {
		t.Setenv(k, t.TempDir())
	}
	t.Setenv("SHERWEB_CLIENT_ID", "")
	t.Setenv("SHERWEB_CLIENT_SECRET", "")
	t.Setenv("SHERWEB_SUBSCRIPTION_KEY", "")
	cfg := &config.Config{
		BaseURL:       "https://api.example.test",
		AuthHeaderVal: "primary-credential",
		Headers:       map[string]string{"X-Config-Header": "config-credential"},
	}
	return New(cfg, 5*time.Second, 0)
}

func redirectTestHop(t *testing.T, method, from, to string) (*http.Request, []*http.Request) {
	t.Helper()
	first, err := http.NewRequest(method, from, nil)
	if err != nil {
		t.Fatal(err)
	}
	next, err := http.NewRequest(method, to, nil)
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range redirectTestCredentials {
		first.Header.Set(k, v)
		next.Header.Set(k, v)
	}
	next.Header.Set("Accept", "application/json")
	return next, []*http.Request{first}
}

func TestRedirectLeavingTheOriginStripsEveryCredential(t *testing.T) {
	c := newRedirectTestClient(t)
	for _, to := range []string{
		"https://elsewhere.example.test/x", // another host
		"https://api.example.test:8443/x",  // same hostname, another port: Go keeps Authorization here
		"http://api.example.test/x",        // https down to http
	} {
		next, via := redirectTestHop(t, http.MethodGet, "https://api.example.test/v1/items", to)
		if err := c.HTTPClient.CheckRedirect(next, via); err != nil {
			t.Fatalf("GET redirect to %s refused: %v", to, err)
		}
		for k := range redirectTestCredentials {
			if v := next.Header.Get(k); v != "" {
				t.Errorf("redirect to %s kept %s", to, k)
			}
		}
		if next.Header.Get("Accept") == "" {
			t.Errorf("redirect to %s also dropped Accept, which carries no credential", to)
		}
	}
}

func TestRedirectLeavingTheOriginRefusesToReplayABody(t *testing.T) {
	c := newRedirectTestClient(t)
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		next, via := redirectTestHop(t, method, "https://api.example.test/v1/items", "https://elsewhere.example.test/v1/items")
		if err := c.HTTPClient.CheckRedirect(next, via); err == nil {
			t.Errorf("a %s was allowed to replay its body to another origin", method)
		}
	}
}

func TestSameOriginRedirectKeepsAuth(t *testing.T) {
	c := newRedirectTestClient(t)
	for _, to := range []string{
		"https://api.example.test/v1/items?page=2",
		"https://api.example.test/v2/items",
		"https://API.Example.TEST/v2/items",     // hostnames compare case-insensitively
		"https://api.example.test:443/v2/items", // an explicit default port is the same origin
	} {
		next, via := redirectTestHop(t, http.MethodGet, "https://api.example.test/v1/items", to)
		next.Header.Del("Authorization")
		if err := c.HTTPClient.CheckRedirect(next, via); err != nil {
			t.Fatalf("same-origin redirect to %s refused: %v", to, err)
		}
		if got := next.Header.Get("Authorization"); got != "primary-credential" {
			t.Errorf("same-origin redirect to %s: Authorization = %q, want the configured credential", to, got)
		}
	}
	// http up to https on the same host is not a downgrade.
	next, via := redirectTestHop(t, http.MethodGet, "http://api.example.test/v1/items", "https://api.example.test/v1/items")
	next.Header.Del("Authorization")
	if err := c.HTTPClient.CheckRedirect(next, via); err != nil || next.Header.Get("Authorization") != "primary-credential" {
		t.Errorf("http to https on the same host: err=%v Authorization=%q", err, next.Header.Get("Authorization"))
	}
}
