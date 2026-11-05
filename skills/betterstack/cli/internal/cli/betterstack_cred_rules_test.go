// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"errors"
	"fmt"
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

	"betterstack-pp-cli/internal/client"
	"betterstack-pp-cli/internal/cliutil/testenv"
)

// api-origin-pinned: the Authorization bearer token reaches only the API
// host, a redirect to another host is refused before it is followed, and the
// configured host still answers.
func TestOriginPinKeepsTheTokenOnTheAPIHost(t *testing.T) {
	const token = "pinUnitBetterstackToken-7e3a9c1f5b"
	var otherHits atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		otherHits.Add(1)
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	t.Cleanup(other.Close)
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+token {
			t.Errorf("the API host got Authorization %q", r.Header.Get("Authorization"))
		}
		if strings.HasSuffix(r.URL.Path, "/redirect") {
			http.Redirect(w, r, other.URL+"/api/v2/monitors", http.StatusTemporaryRedirect)
			return
		}
		_, _ = w.Write([]byte(`{"data":[{"id":"2085431","type":"monitor","attributes":{"pronounceable_name":"api.example.com","status":"up"}}],"pagination":{"first":"","last":"","prev":null,"next":null}}`))
	}))
	t.Cleanup(api.Close)
	testenv.Isolate(t)
	t.Setenv("BETTERSTACK_BASE_URL", api.URL+"/api/v2")
	t.Setenv("BETTERSTACK_API_TOKEN", token)
	flags := rootFlags{timeout: 10 * time.Second}
	c, err := flags.newClient()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Get(context.Background(), "/monitors", nil); err != nil {
		t.Fatalf("configured host: %v", err)
	}
	_, err = c.Get(context.Background(), "/redirect", nil)
	if err == nil || !strings.Contains(err.Error(), "leave the configured Betterstack host") {
		t.Fatalf("redirect: want the pin refusal, got %v", err)
	}
	if otherHits.Load() != 0 {
		t.Fatalf("the other host was contacted %d time(s)", otherHits.Load())
	}
}

// The pin refuses every origin but the API base URL's: another host (Betterstack's
// web app and marketing hosts included), a scheme downgrade, another port and
// a non-ASCII spelling of the host.
func TestPinAllowsOnlyTheAPIOrigin(t *testing.T) {
	pin := normalizedOrigin(&url.URL{Scheme: "https", Host: "uptime.betterstack.com"})
	for raw, ok := range map[string]bool{
		"https://uptime.betterstack.com/api/v2/monitors":     true,
		"https://UPTIME.BetterStack.com:443/api/v2/monitors": true,
		"http://uptime.betterstack.com/api/v2/monitors":      false,
		"https://uptime.betterstack.com:8443/api/v2":         false,
		"https://www.betterstack.com/api/v2/monitors":        false,
		"https://evil.example/api/v2/monitors":               false,
		"https://uptime.bettеrstack.com/api/v2":              false, // Cyrillic e
		"https://uptime.betterstack.com.evil.example/x":      false,
	} {
		req, _ := http.NewRequest(http.MethodGet, raw, nil)
		if err := checkPin(pin, req); (err == nil) != ok {
			t.Errorf("%s: allowed=%v, want %v (%v)", raw, err == nil, ok, err)
		}
	}
	req, _ := http.NewRequest(http.MethodGet, "https://uptime.betterstack.com/api/v2", nil)
	if checkPin("", req) == nil {
		t.Error("an empty pin (unparseable base URL) must refuse everything")
	}
}

// An operator who sets BETTERSTACK_BASE_URL (a proxy in front of the API) moves the
// pin with it: that host is the one origin allowed and the default is refused.
func TestPinFollowsTheConfiguredBaseURL(t *testing.T) {
	c := &client.Client{BaseURL: "https://betterstack-proxy.example.com/api/v2", HTTPClient: &http.Client{}}
	guardClient(c)
	pin, ok := c.HTTPClient.Transport.(originPinTripper)
	if !ok {
		t.Fatalf("the origin pin is not outermost: %T", c.HTTPClient.Transport)
	}
	if pin.pin != "https://betterstack-proxy.example.com:443" {
		t.Fatalf("pin = %q, want the configured origin", pin.pin)
	}
	req, _ := http.NewRequest(http.MethodGet, "https://uptime.betterstack.com/api/v2/monitors", nil)
	if _, err := c.HTTPClient.Do(req); !errors.Is(err, ErrOriginChanged) {
		t.Fatalf("default host under another pin: want ErrOriginChanged, got %v", err)
	}
	unparsed := &client.Client{BaseURL: "https://uptime.betterstack.com:bad/api/v2", HTTPClient: &http.Client{}}
	guardClient(unparsed)
	if upin, ok := unparsed.HTTPClient.Transport.(originPinTripper); !ok || upin.pin != "" {
		t.Fatalf("an unparseable base URL must pin nothing, got %T", unparsed.HTTPClient.Transport)
	}
}

// The pin is the outermost transport, the response cache is off, a second
// hook run does not wrap the chain again, and under --dry-run nothing is sent.
func TestGuardWrapsTheTransport(t *testing.T) {
	base := http.RoundTripper(&http.Transport{})
	c := &client.Client{BaseURL: "https://uptime.betterstack.com/api/v2", HTTPClient: &http.Client{Transport: base}}
	guardClient(c)
	pin, ok := c.HTTPClient.Transport.(originPinTripper)
	if !ok {
		t.Fatalf("the origin pin is not outermost: %T", c.HTTPClient.Transport)
	}
	if pin.next != base {
		t.Fatalf("the pin sends through %T, not the client's transport", pin.next)
	}
	// A client built without a transport sends through the kit's, never the
	// bare default transport.
	bare := &client.Client{BaseURL: "https://uptime.betterstack.com/api/v2", HTTPClient: &http.Client{}}
	guardClient(bare)
	if p, _ := bare.HTTPClient.Transport.(originPinTripper); p.next == nil || p.next == http.RoundTripper(http.DefaultTransport) {
		t.Fatalf("a client built without a transport sends through %T, not the kit's", p.next)
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
	for _, req := range []*http.Request{
		mustRequest(t, http.MethodPost, srv.URL+"/monitors"),
		mustRequest(t, http.MethodPatch, srv.URL+"/heartbeats/90321"),
		mustRequest(t, http.MethodDelete, srv.URL+"/monitors/1187"),
		// A read is refused too: a dry run sends nothing at all.
		mustRequest(t, http.MethodGet, srv.URL+"/monitors"),
	} {
		if _, err := dry.HTTPClient.Do(req); !errors.Is(err, ErrDryRunSend) {
			t.Fatalf("dry run %s %s: want ErrDryRunSend, got %v", req.Method, req.URL.Path, err)
		}
	}
	if hits.Load() != 0 {
		t.Fatal("a --dry-run client sent a request")
	}
}

func mustRequest(t *testing.T, method, raw string) *http.Request {
	t.Helper()
	req, err := http.NewRequest(method, raw, nil)
	if err != nil {
		t.Fatal(err)
	}
	return req
}

func TestNormalizedOriginRefusesNonASCIIAndFillsPorts(t *testing.T) {
	for raw, want := range map[string]string{
		"https://UPTIME.BetterStack.com/api/v2":   "https://uptime.betterstack.com:443",
		"https://uptime.betterstack.com:8443":     "https://uptime.betterstack.com:8443",
		"http://127.0.0.1/x":                      "http://127.0.0.1:80",
		"https://uptime.bettеrstack.com/api/v2/x": "",
		"https://[::1]:9443/api/v2":               "https://[::1]:9443",
	} {
		u, _ := url.Parse(raw)
		if got := normalizedOrigin(u); got != want {
			t.Errorf("%s: got %q, want %q", raw, got, want)
		}
	}
}

// The token from every source reaches the wire as `Authorization: Bearer
// <token>` in this order: the environment (BETTERSTACK_API_TOKEN), then the
// credential store (auth set-token), then a config file's api_token. Holding
// each is the credential kit's (internal/credwire).
func TestTokenFromEverySourceReachesTheWire(t *testing.T) {
	const envTok, storedTok, fileTok = "envUnitToken-2c4e6a8b0d", "storedUnitToken-9f7d5b3a1c", "fileUnitToken-3e5a7c9b1d"
	var got atomic.Value
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.Store(r.Header.Get("Authorization"))
		_, _ = w.Write([]byte(`{"data":[],"pagination":{"next":null}}`))
	}))
	t.Cleanup(api.Close)
	testenv.Isolate(t)
	t.Setenv("BETTERSTACK_BASE_URL", api.URL+"/api/v2")
	t.Setenv("BETTERSTACK_API_TOKEN", "")
	cmd := RootCmd()
	cmd.SetArgs([]string{"auth", "set-token"})
	cmd.SetIn(strings.NewReader(storedTok + "\n"))
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("auth set-token: %v", err)
	}
	flags := rootFlags{timeout: 10 * time.Second}
	wire := func(want, what string) {
		t.Helper()
		c, err := flags.newClient()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := c.Get(context.Background(), "/monitors", nil); err != nil {
			t.Fatal(err)
		}
		if got.Load() != "Bearer "+want {
			t.Fatalf("Authorization on the wire = %v, want Bearer %s", got.Load(), what)
		}
	}
	wire(storedTok, "the stored token")
	t.Setenv("BETTERSTACK_API_TOKEN", envTok)
	c, err := flags.newClient()
	if err != nil {
		t.Fatal(err)
	}
	if c.Config.AuthSource != "env:BETTERSTACK_API_TOKEN" {
		t.Errorf("AuthSource = %q, want env:BETTERSTACK_API_TOKEN", c.Config.AuthSource)
	}
	wire(envTok, "BETTERSTACK_API_TOKEN (it wins over the stored token)")
	// A 0600 config file's api_token (BETTERSTACK_CONFIG), no environment token.
	cfgPath := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(cfgPath, []byte(fmt.Sprintf("api_token = %q\n", fileTok)), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BETTERSTACK_API_TOKEN", "")
	t.Setenv("BETTERSTACK_CONFIG", cfgPath)
	wire(fileTok, "the config file's api_token")
}
