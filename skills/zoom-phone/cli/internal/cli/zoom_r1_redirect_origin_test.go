// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"zoom-phone-pp-cli/internal/client"
	"zoom-phone-pp-cli/internal/config"
)

// zoom-phone hand-fix: zoom-phone-same-origin-redirects (class
// credential-host-redirect, Codex review r1 of the onboard). Every HTTP client
// that carries a credential or a body follows a redirect only to the origin
// it started on, and never redirects a request that is not GET or HEAD.

// zoomR1Hosts is an in-memory transport for several hosts, so a redirect to a
// public address is observable without any network.
type zoomR1Hosts struct {
	mu      sync.Mutex
	hits    []string // "METHOD host/path body"
	handler func(w http.ResponseWriter, r *http.Request)
}

func (h *zoomR1Hosts) RoundTrip(req *http.Request) (*http.Response, error) {
	var body []byte
	if req.Body != nil {
		body, _ = io.ReadAll(req.Body)
		_ = req.Body.Close()
	}
	h.mu.Lock()
	h.hits = append(h.hits, req.Method+" "+req.URL.Host+req.URL.Path+" "+string(body))
	h.mu.Unlock()
	rec := httptest.NewRecorder()
	h.handler(rec, req)
	resp := rec.Result()
	resp.Request = req
	return resp, nil
}

func (h *zoomR1Hosts) hitsOn(host string) []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	var out []string
	for _, hit := range h.hits {
		if strings.Contains(hit, " "+host+"/") {
			out = append(out, hit)
		}
	}
	return out
}

func zoomR1HostsClient(t *testing.T, cfg *config.Config, hosts *zoomR1Hosts) *client.Client {
	t.Helper()
	localTestEnv(t)
	c := client.New(cfg, 5*time.Second, 0)
	c.NoCache = true
	c.HTTPClient.Transport = hosts
	if err := ApplyClientHooks(c); err != nil {
		t.Fatal(err)
	}
	return c
}

// The round-1 input: `common-areas update-pin-code CA --pin-code 679412`
// answered `307 Location: https://example.com/collect` replayed the PIN body
// to that host (Go drops Authorization on the hop but keeps the body).
func TestZoomR1APIClientNeverReplaysABodyOffOrigin(t *testing.T) {
	const collector = "203.0.113.7"
	hosts := &zoomR1Hosts{handler: func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Host == "api.zoom.us" {
			w.Header().Set("Location", "https://"+collector+"/collect?k=leak-marker-0001")
			w.WriteHeader(http.StatusTemporaryRedirect)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{}`)
	}}
	c := zoomR1HostsClient(t, &config.Config{BaseURL: "https://api.zoom.us/v2", AccessToken: "redirect-test-token-0001"}, hosts)
	_, _, err := c.PatchWithParams(context.Background(), "/phone/common_areas/CA/pin_code", nil, map[string]any{"pin_code": "679412"})
	if got := hosts.hitsOn(collector); len(got) > 0 {
		t.Fatalf("the PATCH body was replayed off the Zoom origin: %q", got)
	}
	if err == nil {
		t.Fatal("a redirected PATCH must fail")
	}
	if strings.Contains(err.Error(), "leak-marker-0001") || strings.Contains(err.Error(), collector) {
		t.Fatalf("the refusal echoes the Location: %v", err)
	}
}

// A same-origin redirect of a write is refused too: its body would be replayed.
func TestZoomR1APIClientNeverRedirectsAWrite(t *testing.T) {
	hosts := &zoomR1Hosts{handler: func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/phone/common_areas/CA/pin_code" {
			w.Header().Set("Location", "https://api.zoom.us/v2/elsewhere")
			w.WriteHeader(http.StatusPermanentRedirect)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{}`)
	}}
	c := zoomR1HostsClient(t, &config.Config{BaseURL: "https://api.zoom.us/v2", AccessToken: "redirect-test-token-0002"}, hosts)
	_, _, err := c.PatchWithParams(context.Background(), "/phone/common_areas/CA/pin_code", nil, map[string]any{"pin_code": "679412"})
	if err == nil {
		t.Fatal("a redirected PATCH must fail")
	}
	for _, hit := range hosts.hitsOn("api.zoom.us") {
		if strings.Contains(hit, "/elsewhere") {
			t.Fatalf("the write was redirected: %q", hit)
		}
	}
}

// Reads follow a redirect only within the origin they started on.
func TestZoomR1APIClientReadsStayOnOrigin(t *testing.T) {
	hosts := &zoomR1Hosts{handler: func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Host == "api.zoom.us" && r.URL.Path == "/v2/phone/users":
			w.Header().Set("Location", "https://api.zoom.us/v2/phone/users2")
			w.WriteHeader(http.StatusFound)
		case r.URL.Host == "api.zoom.us" && r.URL.Path == "/v2/phone/sites":
			w.Header().Set("Location", "https://203.0.113.8/v2/phone/sites")
			w.WriteHeader(http.StatusFound)
		default:
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"ok":true}`)
		}
	}}
	c := zoomR1HostsClient(t, &config.Config{BaseURL: "https://api.zoom.us/v2", AccessToken: "redirect-test-token-0003"}, hosts)
	if _, err := c.Get(context.Background(), "/phone/users", nil); err != nil {
		t.Fatalf("a same-origin read redirect must be followed: %v", err)
	}
	if _, err := c.Get(context.Background(), "/phone/sites", nil); err == nil {
		t.Fatal("a cross-origin read redirect must be refused")
	}
	if got := hosts.hitsOn("203.0.113.8"); len(got) > 0 {
		t.Fatalf("the read followed a redirect off the Zoom origin: %q", got)
	}
}

// The token mint never follows a redirect, even within its origin: a 307
// replays the form (account ID) and the Basic client secret.
func TestZoomR1TokenMintNeverRedirects(t *testing.T) {
	hosts := &zoomR1Hosts{handler: func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/oauth/token" {
			w.Header().Set("Location", "https://zoom.us/oauth/token2")
			w.WriteHeader(http.StatusTemporaryRedirect)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"minted-after-redirect","expires_in":3600}`)
	}}
	cfg := &config.Config{BaseURL: "https://api.zoom.us/v2", TokenURL: "https://zoom.us/oauth/token", ClientID: "cid-r1", ClientSecret: "client-secret-r1-0002", ZoomPhoneAccountId: "acct-r1"}
	c := zoomR1HostsClient(t, cfg, hosts)
	_, _ = c.Get(context.Background(), "/phone/users", nil)
	if got := hosts.hitsOn("zoom.us"); len(got) != 1 {
		t.Fatalf("the token request was redirected: %q", got)
	}

	// auth login's token client.
	hosts2 := &zoomR1Hosts{handler: hosts.handler}
	_, err := mintClientCredentialsToken(&http.Client{Transport: hosts2}, "https://zoom.us/oauth/token", "cid-r1", "client-secret-r1-0003", "acct-r1")
	if err == nil {
		t.Fatal("auth login must refuse a redirected token response")
	}
	if got := hosts2.hitsOn("zoom.us"); len(got) != 1 {
		t.Fatalf("auth login's token request was redirected: %q", got)
	}
}

// The feedback POST carries a body to a configured endpoint: never redirected.
func TestZoomR1FeedbackNeverRedirects(t *testing.T) {
	var targetHits int
	var mu sync.Mutex
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		targetHits++
		mu.Unlock()
	}))
	t.Cleanup(target.Close)
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/collect", http.StatusTemporaryRedirect)
	}))
	t.Cleanup(origin.Close)
	if err := postFeedback(origin.URL+"/feedback", FeedbackEntry{Text: "hello"}); err == nil {
		t.Fatal("a redirected feedback POST must fail")
	}
	mu.Lock()
	defer mu.Unlock()
	if targetHits != 0 {
		t.Fatalf("the feedback body was replayed to the redirect target (%d hits)", targetHits)
	}
}
