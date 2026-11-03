// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"watchguard-pp-cli/internal/config"
)

// dialAttackerTo routes attacker.example:80 (a hostname, so the private-IP
// redirect guard does not apply) to a local server standing in for another
// origin.
func dialAttackerTo(c *Client, other *httptest.Server) {
	c.HTTPClient.Transport = &http.Transport{DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
		if addr == "attacker.example:80" {
			addr = other.Listener.Addr().String()
		}
		return (&net.Dialer{}).DialContext(ctx, network, addr)
	}}
}

// hand-fix (handfixes.json redirect-refuses-cross-origin-body, ported from
// connectsecure): a 307/308 replays the request body, so a write carrying a
// BOVPN pre-shared key must not follow a redirect to another origin.
func TestWriteBodyNotReplayedCrossOrigin(t *testing.T) {
	for _, code := range []int{http.StatusTemporaryRedirect, http.StatusPermanentRedirect} {
		got := make(chan string, 1)
		other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			got <- r.Method
			_, _ = w.Write([]byte(`{}`))
		}))
		origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "http://attacker.example"+r.URL.Path, code)
		}))
		c := New(&config.Config{BaseURL: origin.URL, AccessToken: "tok"}, 5*time.Second, 0)
		c.NoCache = true
		dialAttackerTo(c, other)
		_, _, err := c.PostWithParamsAndHeaders(context.Background(), "/rest/firebox/management/v1/configuration/ACC-1/bovpn/ipsec/tunnels", nil, map[string]any{"name": "t1", "psk": "SENTINEL-PSK"}, nil)
		select {
		case m := <-got:
			t.Errorf("%d: a %s body followed a cross-origin redirect", code, m)
		default:
		}
		if err == nil {
			t.Errorf("%d: cross-origin body redirect returned no error", code)
		}
		other.Close()
		origin.Close()
	}
}

// For 301/302/303 Go rewrites the write into a bodiless GET before
// CheckRedirect runs, so the write never lands and must not read as success.
func TestWriteNotDowngradedCrossOrigin(t *testing.T) {
	for _, code := range []int{http.StatusMovedPermanently, http.StatusFound, http.StatusSeeOther} {
		got := make(chan string, 1)
		other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			got <- r.Method
			_, _ = w.Write([]byte(`{"id":"9"}`))
		}))
		origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "http://attacker.example"+r.URL.Path, code)
		}))
		c := New(&config.Config{BaseURL: origin.URL, AccessToken: "tok"}, 5*time.Second, 0)
		c.NoCache = true
		dialAttackerTo(c, other)
		_, _, err := c.PostWithParamsAndHeaders(context.Background(), "/rest/firebox/management/v1/configuration/ACC-1/bovpn/ipsec/tunnels", nil, map[string]any{"psk": "SENTINEL-PSK"}, nil)
		select {
		case m := <-got:
			t.Errorf("%d: a POST reached the other origin as %s", code, m)
		default:
		}
		if err == nil {
			t.Errorf("%d: a cross-origin write redirect returned no error", code)
		}
		other.Close()
		origin.Close()
	}
}

// On one origin a 301/302 also replays the write as a GET, so it never
// landed; a 303 (the write was applied, fetch the result) is still followed.
func TestWriteNotDowngradedSameOrigin(t *testing.T) {
	for code, wantErr := range map[int]bool{http.StatusMovedPermanently: true, http.StatusFound: true, http.StatusSeeOther: false} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/landed" {
				_, _ = w.Write([]byte(`{"id":"9"}`))
				return
			}
			http.Redirect(w, r, "/landed", code)
		}))
		c := New(&config.Config{BaseURL: srv.URL, AccessToken: "tok"}, 5*time.Second, 0)
		c.NoCache = true
		_, _, err := c.PostWithParamsAndHeaders(context.Background(), "/rest/firebox/management/v1/configuration/ACC-1/aliases", nil, map[string]any{"name": "x"}, nil)
		if (err != nil) != wantErr {
			t.Errorf("%d: err = %v, want error %v", code, err, wantErr)
		}
		srv.Close()
	}
}

// After a same-origin 303 the write landed and the follow-up is a plain GET,
// so a later 302 is not a failure.
func TestWriteRedirectChainChecksTheLastHop(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rest/x":
			http.Redirect(w, r, "/step1", http.StatusSeeOther)
		case "/step1":
			http.Redirect(w, r, "/landed", http.StatusFound)
		default:
			_, _ = w.Write([]byte(`{"id":"9"}`))
		}
	}))
	defer srv.Close()
	c := New(&config.Config{BaseURL: srv.URL, AccessToken: "tok"}, 5*time.Second, 0)
	c.NoCache = true
	if _, _, err := c.PostWithParamsAndHeaders(context.Background(), "/rest/x", nil, map[string]any{"name": "x"}, nil); err != nil {
		t.Fatalf("303 then 302 after an applied write: %v", err)
	}
}

// hand-fix (handfixes.json redirect-strips-configured-headers, the generic
// half of connectsecure's redirect-strips-client-auth-token): a header set in
// config.toml [headers] exists for the API origin only, whatever its name,
// and Go forwards custom headers to a redirect target on another host.
func TestConfiguredHeaderDroppedOnCrossOriginRedirect(t *testing.T) {
	got := make(chan string, 1)
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got <- r.Header.Get("X-Proxy-Token") + r.Header.Get("X-Tenant-Signature") + r.Header.Get("WatchGuard-API-Key") + r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer other.Close()
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://attacker.example"+r.URL.Path, http.StatusFound)
	}))
	defer origin.Close()
	c := New(&config.Config{BaseURL: origin.URL, AccessToken: "tok", WatchguardApiKey: "KEY", Headers: map[string]string{"X-Proxy-Token": "HDRSECRET_42", "X-Tenant-Signature": "SIG_42"}}, 5*time.Second, 0)
	c.NoCache = true
	dialAttackerTo(c, other)
	_, _ = c.Get(context.Background(), "/rest/probe", nil)
	select {
	case h := <-got:
		if h != "" {
			t.Fatalf("a configured header followed a cross-origin redirect: %q", h)
		}
	default:
		t.Fatal("redirect was not followed; the test did not exercise the cross-origin hop")
	}
}
