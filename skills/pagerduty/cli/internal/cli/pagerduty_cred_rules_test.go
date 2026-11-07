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

	"pagerduty-pp-cli/internal/client"
	"pagerduty-pp-cli/internal/cliutil/testenv"
)

// api-origin-pinned: the `Token token=` credential reaches only the API
// host, a redirect to another host is refused before it is followed, and the
// configured host still answers.
func TestOriginPinKeepsTheTokenOnTheAPIHost(t *testing.T) {
	if inFreshProcess(t) {
		return
	}
	const token = "pinUnitPagerDutyToken-7e3a9c1f5b"
	var otherHits atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		otherHits.Add(1)
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	t.Cleanup(other.Close)
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Token token="+token {
			t.Errorf("the API host got Authorization %q", r.Header.Get("Authorization"))
		}
		if strings.HasSuffix(r.URL.Path, "/redirect") {
			http.Redirect(w, r, other.URL+"/v1/agent", http.StatusTemporaryRedirect)
			return
		}
		_, _ = w.Write([]byte(`{"data":[{"id":"2085431","type":"monitor","attributes":{"pronounceable_name":"api.example.com","status":"up"}}],"pagination":{"first":"","last":"","prev":null,"next":null}}`))
	}))
	t.Cleanup(api.Close)
	testenv.Isolate(t)
	t.Setenv("PAGERDUTY_BASE_URL", api.URL+"/v1")
	t.Setenv("PAGERDUTY_API_KEY", token)
	flags := rootFlags{timeout: 10 * time.Second}
	c, err := flags.newClient()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Get(context.Background(), "/agent", nil); err != nil {
		t.Fatalf("configured host: %v", err)
	}
	_, err = c.Get(context.Background(), "/redirect", nil)
	if err == nil || !strings.Contains(err.Error(), "leave the configured PagerDuty host") {
		t.Fatalf("redirect: want the pin refusal, got %v", err)
	}
	if otherHits.Load() != 0 {
		t.Fatalf("the other host was contacted %d time(s)", otherHits.Load())
	}
}

// The pin refuses every origin but the API base URL's: another host (PagerDuty's
// web app and marketing hosts included), a scheme downgrade, another port and
// a non-ASCII spelling of the host.
func TestPinAllowsOnlyTheAPIOrigin(t *testing.T) {
	pin := normalizedOrigin(&url.URL{Scheme: "https", Host: "api.pagerduty.com"})
	for raw, ok := range map[string]bool{
		"https://api.pagerduty.com/agent":          true,
		"https://API.PagerDuty.com:443/v1/agent":   true,
		"http://api.pagerduty.com/v1/agent":        false,
		"https://api.pagerduty.com:8443/v1":        false,
		"https://www.pagerduty.com/v1/agent":       false,
		"https://evil.example/v1/agent":            false,
		"https://api.pagerdutу.com/v1":             false, // Cyrillic u
		"https://api.pagerduty.com.evil.example/x": false,
	} {
		req, _ := http.NewRequest(http.MethodGet, raw, nil)
		if err := checkPin(pin, req); (err == nil) != ok {
			t.Errorf("%s: allowed=%v, want %v (%v)", raw, err == nil, ok, err)
		}
	}
	req, _ := http.NewRequest(http.MethodGet, "https://api.pagerduty.com", nil)
	if checkPin("", req) == nil {
		t.Error("an empty pin (unparseable base URL) must refuse everything")
	}
}

// An operator who sets PAGERDUTY_BASE_URL (a proxy in front of the API) moves the
// pin with it: that host is the one origin allowed and the default is refused.
func TestPinFollowsTheConfiguredBaseURL(t *testing.T) {
	c := &client.Client{BaseURL: "https://pagerduty-proxy.example.com/v1", HTTPClient: &http.Client{}}
	guardClient(c)
	pin, ok := c.HTTPClient.Transport.(originPinTripper)
	if !ok {
		t.Fatalf("the origin pin is not outermost: %T", c.HTTPClient.Transport)
	}
	if pin.pin != "https://pagerduty-proxy.example.com:443" {
		t.Fatalf("pin = %q, want the configured origin", pin.pin)
	}
	req, _ := http.NewRequest(http.MethodGet, "https://api.pagerduty.com/agent", nil)
	if _, err := c.HTTPClient.Do(req); !errors.Is(err, ErrOriginChanged) {
		t.Fatalf("default host under another pin: want ErrOriginChanged, got %v", err)
	}
	unparsed := &client.Client{BaseURL: "https://api.pagerduty.com:bad/v1", HTTPClient: &http.Client{}}
	guardClient(unparsed)
	if upin, ok := unparsed.HTTPClient.Transport.(originPinTripper); !ok || upin.pin != "" {
		t.Fatalf("an unparseable base URL must pin nothing, got %T", unparsed.HTTPClient.Transport)
	}
}

// The pin is the outermost transport, the response cache is off, a second
// hook run does not wrap the chain again, and under --dry-run nothing is sent.
func TestGuardWrapsTheTransport(t *testing.T) {
	c := &client.Client{BaseURL: "https://api.pagerduty.com", HTTPClient: &http.Client{}}
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
	for _, req := range []*http.Request{
		mustRequest(t, http.MethodPost, srv.URL+"/agent"),
		mustRequest(t, http.MethodPatch, srv.URL+"/heartbeats/90321"),
		mustRequest(t, http.MethodDelete, srv.URL+"/monitors/1187"),
		// A read is refused too: a dry run sends nothing at all.
		mustRequest(t, http.MethodGet, srv.URL+"/agent"),
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
		"https://API.PagerDuty.com/v1":   "https://api.pagerduty.com:443",
		"https://api.pagerduty.com:8443": "https://api.pagerduty.com:8443",
		"http://127.0.0.1/x":             "http://127.0.0.1:80",
		"https://api.pagerdutу.com/v1/x": "",
		"https://[::1]:9443/v1":          "https://[::1]:9443",
	} {
		u, _ := url.Parse(raw)
		if got := normalizedOrigin(u); got != want {
			t.Errorf("%s: got %q, want %q", raw, got, want)
		}
	}
}

// The token from every source goes on the wire as
// `Authorization: Token token=<key>`: the environment (PAGERDUTY_API_KEY), the
// credential store (auth set-token) and a config file's api_key, and the
// environment wins. Holding each is the credential kit's (internal/credwire's
// built-binary tests).
func TestTokenFromEverySourceIsHeld(t *testing.T) {
	if inFreshProcess(t) {
		return
	}
	const envTok, storedTok, fileTok = "envUnitToken-2c4e6a8b0d", "storedUnitToken-9f7d5b3a1c", "fileUnitToken-3e5a7c9b1d"
	var got atomic.Value
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.Store(strings.TrimPrefix(r.Header.Get("Authorization"), "Token token="))
		_, _ = w.Write([]byte(`{"data":[],"pagination":{"next":null}}`))
	}))
	t.Cleanup(api.Close)
	testenv.Isolate(t)
	t.Setenv("PAGERDUTY_BASE_URL", api.URL+"/v1")
	t.Setenv("PAGERDUTY_API_KEY", "")
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
		if _, err := c.Get(context.Background(), "/agent", nil); err != nil {
			t.Fatal(err)
		}
		if got.Load() != want {
			t.Fatalf("token on the wire = %v, want %s", got.Load(), what)
		}
	}
	wire(storedTok, "the stored token")
	t.Setenv("PAGERDUTY_API_KEY", envTok)
	c, err := flags.newClient()
	if err != nil {
		t.Fatal(err)
	}
	if c.Config.AuthSource != "env:PAGERDUTY_API_KEY" {
		t.Errorf("AuthSource = %q, want env:PAGERDUTY_API_KEY", c.Config.AuthSource)
	}
	wire(envTok, "PAGERDUTY_API_KEY (it wins over the stored token)")
	// A 0600 config file's api_key (PAGERDUTY_CONFIG), no environment token.
	cfgPath := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(cfgPath, []byte(fmt.Sprintf("api_key = %q\n", fileTok)), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PAGERDUTY_API_KEY", "")
	t.Setenv("PAGERDUTY_CONFIG", cfgPath)
	wire(fileTok, "the config file's api_key")
}
