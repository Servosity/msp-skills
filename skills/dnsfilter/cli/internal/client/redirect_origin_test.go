// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written for the reprint review (handfixes.json: redirect-stays-on-origin,
// redirect-never-forwards-credentials).

package client

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"dnsfilter-pp-cli/internal/cliutil"
	"dnsfilter-pp-cli/internal/cliutil/testenv"
	"dnsfilter-pp-cli/internal/config"
)

const redirectTestKey = "test-key-000000000000"

// Every hop whose scheme, host or port differs from the request it follows is
// refused, whatever the address carries. Codex review round 2: a cross-host
// `Location: https://example.com/collect?api_key=<key>` reached example.com
// with the key in the URL after every header strip.
func TestRedirectRefusesEveryCrossOriginHop(t *testing.T) {
	testenv.Isolate(t, cliutil.CacheDir)
	var mu sync.Mutex
	var seen []string
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, r.Host+r.URL.RequestURI())
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(other.Close)
	t.Cleanup(swapDialResolve(func(ctx context.Context, host string) ([]net.IP, error) {
		if host == "collect.cross.example" {
			return []net.IP{net.ParseIP("203.0.113.20")}, nil
		}
		return net.DefaultResolver.LookupIP(ctx, "ip", host)
	}))
	t.Cleanup(swapDialPinned(func(ctx context.Context, network, addr string) (net.Conn, error) {
		if h, p, err := net.SplitHostPort(addr); err == nil && h == "203.0.113.20" {
			addr = net.JoinHostPort("127.0.0.1", p)
		}
		return (&net.Dialer{}).DialContext(ctx, network, addr)
	}))
	otherPort := other.URL[strings.LastIndex(other.URL, ":")+1:]
	cross := "http://collect.cross.example:" + otherPort
	targets := map[string]string{
		"/v1/query":    cross + "/collect?api_key=" + redirectTestKey,
		"/v1/path":     cross + "/collect/" + redirectTestKey,
		"/v1/fragment": cross + "/collect#" + redirectTestKey,
		"/v1/userinfo": "http://u:" + redirectTestKey + "@collect.cross.example:" + otherPort + "/collect",
		"/v1/port":     "http://127.0.0.1:" + otherPort + "/collect",
		"/v1/scheme":   "", // filled below: the origin's own host and port over https
	}
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if loc, ok := targets[r.URL.Path]; ok {
			w.Header().Set("Location", loc)
			w.WriteHeader(http.StatusFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(origin.Close)
	targets["/v1/scheme"] = strings.Replace(origin.URL, "http://", "https://", 1) + "/v1/landed"
	c := New(&config.Config{BaseURL: origin.URL, DnsfilterApiKey: redirectTestKey}, 5*time.Second, 0)
	c.NoCache = true
	for path := range targets {
		_, err := c.Get(context.Background(), path, nil)
		if !errors.Is(err, ErrRedirectCrossOrigin) {
			t.Errorf("GET %s: a cross-origin redirect was not refused as one: %v", path, err)
		}
		if err != nil && strings.Contains(err.Error(), redirectTestKey) {
			t.Errorf("GET %s: the refusal echoed the key: %v", path, err)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	for _, s := range seen {
		t.Errorf("another origin received a request: %s", s)
	}
}

// A same-origin hop is followed, but carries no credential: no Authorization,
// no config-file header, no per-request header outside the allowlist. The
// vendor documents no redirect (spec.json has no 3xx response), so nothing
// requires the key on a hop.
func TestRedirectSameOriginCarriesNoCredential(t *testing.T) {
	testenv.Isolate(t, cliutil.CacheDir)
	var mu sync.Mutex
	var landed http.Header
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/hop":
			w.Header().Set("Location", "/v1/landed")
			w.WriteHeader(http.StatusFound)
		case "/v1/landed":
			mu.Lock()
			landed = r.Header.Clone()
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"id":"1"}}`))
		}
	}))
	t.Cleanup(origin.Close)
	cfg := &config.Config{
		BaseURL:         origin.URL,
		DnsfilterApiKey: redirectTestKey,
		Headers:         map[string]string{"X-Api-Key": "proxy-secret-origin", "X-Org-Id": "42"},
	}
	c := New(cfg, 5*time.Second, 0)
	c.NoCache = true
	body, err := c.GetWithHeaders(context.Background(), "/v1/hop", nil, map[string]string{"X-Session-Token": "session-origin", "Accept": "application/json"})
	if err != nil {
		t.Fatalf("a same-origin redirect was refused: %v", err)
	}
	if !strings.Contains(string(body), `"id":"1"`) {
		t.Fatalf("same-origin redirect returned %s, want the landed body", body)
	}
	mu.Lock()
	defer mu.Unlock()
	if landed == nil {
		t.Fatal("the same-origin hop was not followed")
	}
	for _, name := range []string{"Authorization", "X-Api-Key", "X-Org-Id", "X-Session-Token", "Cookie", "Referer"} {
		if v := landed.Get(name); v != "" {
			t.Errorf("%s was forwarded on a same-origin redirect: %q", name, v)
		}
	}
	if landed.Get("Accept") == "" || landed.Get("User-Agent") == "" {
		t.Errorf("an allowlisted non-credential header was dropped: %v", landed)
	}
}

// A hop carries no key, so its 401 says nothing about the key: the error names
// the redirect and never reads as "HTTP 401" (which the CLI and MCP answer with
// "check your API key").
func TestRedirectedAuthFailureNamesTheRedirect(t *testing.T) {
	testenv.Isolate(t, cliutil.CacheDir)
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/needs-auth":
			w.Header().Set("Location", "/v1/protected")
			w.WriteHeader(http.StatusFound)
		case "/v1/protected":
			w.Header().Set("Content-Type", "application/json")
			if r.Header.Get("Authorization") == "" {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"error":"Not Authorized"}`))
				return
			}
			_, _ = w.Write([]byte(`{"data":{"id":"1"}}`))
		}
	}))
	t.Cleanup(origin.Close)
	c := New(&config.Config{BaseURL: origin.URL, DnsfilterApiKey: redirectTestKey}, 5*time.Second, 0)
	c.NoCache = true
	_, err := c.Get(context.Background(), "/v1/needs-auth", nil)
	if !errors.Is(err, ErrRedirectAuthWithheld) {
		t.Fatalf("a 401 behind a redirect was not named as one: %v", err)
	}
	if msg := err.Error(); strings.Contains(msg, "HTTP 401") || strings.Contains(msg, redirectTestKey) || !strings.Contains(msg, "/v1/protected") {
		t.Fatalf("error = %q, want the redirect target named, no \"HTTP 401\", no key", msg)
	}
}
