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

	"domotz-pp-cli/internal/client"
	"domotz-pp-cli/internal/cliutil/testenv"
)

// api-origin-pinned: the X-Api-Key credential reaches only the API
// host, a redirect to another host is refused before it is followed, and the
// configured host still answers.
func TestOriginPinKeepsTheTokenOnTheAPIHost(t *testing.T) {
	const token = "pinUnitDomotzToken-7e3a9c1f5b"
	var otherHits atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		otherHits.Add(1)
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	t.Cleanup(other.Close)
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Api-Key") != token {
			t.Errorf("the API host got X-Api-Key %q", r.Header.Get("X-Api-Key"))
		}
		if strings.HasSuffix(r.URL.Path, "/redirect") {
			http.Redirect(w, r, other.URL+"/public-api/v1/agent", http.StatusTemporaryRedirect)
			return
		}
		_, _ = w.Write([]byte(`{"data":[{"id":"2085431","type":"monitor","attributes":{"pronounceable_name":"api.example.com","status":"up"}}],"pagination":{"first":"","last":"","prev":null,"next":null}}`))
	}))
	t.Cleanup(api.Close)
	testenv.Isolate(t)
	t.Setenv("DOMOTZ_BASE_URL", api.URL+"/public-api/v1")
	t.Setenv("DOMOTZ_API_KEY", token)
	flags := rootFlags{timeout: 10 * time.Second}
	c, err := flags.newClient()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Get(context.Background(), "/agent", nil); err != nil {
		t.Fatalf("configured host: %v", err)
	}
	_, err = c.Get(context.Background(), "/redirect", nil)
	if err == nil || !strings.Contains(err.Error(), "leave the configured Domotz host") {
		t.Fatalf("redirect: want the pin refusal, got %v", err)
	}
	if otherHits.Load() != 0 {
		t.Fatalf("the other host was contacted %d time(s)", otherHits.Load())
	}
}

// The pin refuses every origin but the API base URL's: another host (Domotz's
// web app and marketing hosts included), a scheme downgrade, another port and
// a non-ASCII spelling of the host.
func TestPinAllowsOnlyTheAPIOrigin(t *testing.T) {
	pin := normalizedOrigin(&url.URL{Scheme: "https", Host: "api-us-east-1-cell-1.domotz.com"})
	for raw, ok := range map[string]bool{
		"https://api-us-east-1-cell-1.domotz.com/public-api/v1/agent":     true,
		"https://API-US-EAST-1-CELL-1.Domotz.com:443/public-api/v1/agent": true,
		"http://api-us-east-1-cell-1.domotz.com/public-api/v1/agent":      false,
		"https://api-us-east-1-cell-1.domotz.com:8443/public-api/v1":      false,
		"https://www.domotz.com/public-api/v1/agent":                      false,
		"https://evil.example/public-api/v1/agent":                        false,
		"https://api-us-еast-1-cell-1.domotz.com/public-api/v1":           false, // Cyrillic e
		"https://api-us-east-1-cell-1.domotz.com.evil.example/x":          false,
	} {
		req, _ := http.NewRequest(http.MethodGet, raw, nil)
		if err := checkPin(pin, req); (err == nil) != ok {
			t.Errorf("%s: allowed=%v, want %v (%v)", raw, err == nil, ok, err)
		}
	}
	req, _ := http.NewRequest(http.MethodGet, "https://api-us-east-1-cell-1.domotz.com/public-api/v1", nil)
	if checkPin("", req) == nil {
		t.Error("an empty pin (unparseable base URL) must refuse everything")
	}
}

// An operator who sets DOMOTZ_BASE_URL (a proxy in front of the API) moves the
// pin with it: that host is the one origin allowed and the default is refused.
func TestPinFollowsTheConfiguredBaseURL(t *testing.T) {
	c := &client.Client{BaseURL: "https://domotz-proxy.example.com/public-api/v1", HTTPClient: &http.Client{}}
	guardClient(c)
	pin, ok := c.HTTPClient.Transport.(originPinTripper)
	if !ok {
		t.Fatalf("the origin pin is not outermost: %T", c.HTTPClient.Transport)
	}
	if pin.pin != "https://domotz-proxy.example.com:443" {
		t.Fatalf("pin = %q, want the configured origin", pin.pin)
	}
	req, _ := http.NewRequest(http.MethodGet, "https://api-us-east-1-cell-1.domotz.com/public-api/v1/agent", nil)
	if _, err := c.HTTPClient.Do(req); !errors.Is(err, ErrOriginChanged) {
		t.Fatalf("default host under another pin: want ErrOriginChanged, got %v", err)
	}
	unparsed := &client.Client{BaseURL: "https://api-us-east-1-cell-1.domotz.com:bad/public-api/v1", HTTPClient: &http.Client{}}
	guardClient(unparsed)
	if upin, ok := unparsed.HTTPClient.Transport.(originPinTripper); !ok || upin.pin != "" {
		t.Fatalf("an unparseable base URL must pin nothing, got %T", unparsed.HTTPClient.Transport)
	}
}

// The pin is the outermost transport, the response cache is off, a second
// hook run does not wrap the chain again, and under --dry-run nothing is sent.
func TestGuardWrapsTheTransport(t *testing.T) {
	base := http.RoundTripper(&http.Transport{})
	c := &client.Client{BaseURL: "https://api-us-east-1-cell-1.domotz.com/public-api/v1", HTTPClient: &http.Client{Transport: base}}
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
	bare := &client.Client{BaseURL: "https://api-us-east-1-cell-1.domotz.com/public-api/v1", HTTPClient: &http.Client{}}
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
		"https://API-US-EAST-1-CELL-1.Domotz.com/public-api/v1": "https://api-us-east-1-cell-1.domotz.com:443",
		"https://api-us-east-1-cell-1.domotz.com:8443":          "https://api-us-east-1-cell-1.domotz.com:8443",
		"http://127.0.0.1/x": "http://127.0.0.1:80",
		"https://api-us-еast-1-cell-1.domotz.com/public-api/v1/x": "",
		"https://[::1]:9443/public-api/v1":                        "https://[::1]:9443",
	} {
		u, _ := url.Parse(raw)
		if got := normalizedOrigin(u); got != want {
			t.Errorf("%s: got %q, want %q", raw, got, want)
		}
	}
}

// The key from every source reaches the wire as `X-Api-Key: <key>` (raw, no
// scheme) in this order: the environment (DOMOTZ_API_KEY), then the credential
// store (auth set-token), then a config file's api_key. Holding each is the
// credential kit's (internal/credwire).
func TestTokenFromEverySourceReachesTheWire(t *testing.T) {
	const envTok, storedTok, fileTok = "envUnitToken-2c4e6a8b0d", "storedUnitToken-9f7d5b3a1c", "fileUnitToken-3e5a7c9b1d"
	var got atomic.Value
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.Store(r.Header.Get("X-Api-Key"))
		_, _ = w.Write([]byte(`{"data":[],"pagination":{"next":null}}`))
	}))
	t.Cleanup(api.Close)
	testenv.Isolate(t)
	t.Setenv("DOMOTZ_BASE_URL", api.URL+"/public-api/v1")
	t.Setenv("DOMOTZ_API_KEY", "")
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
			t.Fatalf("X-Api-Key on the wire = %v, want %s", got.Load(), what)
		}
	}
	wire(storedTok, "the stored token")
	t.Setenv("DOMOTZ_API_KEY", envTok)
	c, err := flags.newClient()
	if err != nil {
		t.Fatal(err)
	}
	if c.Config.AuthSource != "env:DOMOTZ_API_KEY" {
		t.Errorf("AuthSource = %q, want env:DOMOTZ_API_KEY", c.Config.AuthSource)
	}
	wire(envTok, "DOMOTZ_API_KEY (it wins over the stored token)")
	// A 0600 config file's api_key (DOMOTZ_CONFIG), no environment token.
	cfgPath := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(cfgPath, []byte(fmt.Sprintf("api_key = %q\n", fileTok)), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DOMOTZ_API_KEY", "")
	t.Setenv("DOMOTZ_CONFIG", cfgPath)
	wire(fileTok, "the config file's api_key")
}
