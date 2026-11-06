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

	"knowbe4-pp-cli/internal/client"
	"knowbe4-pp-cli/internal/cliutil/testenv"
)

// api-origin-pinned: the Bearer credential reaches only the API
// host, a redirect to another host is refused before it is followed, and the
// configured host still answers.
func TestOriginPinKeepsTheTokenOnTheAPIHost(t *testing.T) {
	const token = "pinUnitKnowBe4Token-7e3a9c1f5b"
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
			http.Redirect(w, r, other.URL+"/v1/agent", http.StatusTemporaryRedirect)
			return
		}
		_, _ = w.Write([]byte(`{"data":[{"id":"2085431","type":"monitor","attributes":{"pronounceable_name":"api.example.com","status":"up"}}],"pagination":{"first":"","last":"","prev":null,"next":null}}`))
	}))
	t.Cleanup(api.Close)
	testenv.Isolate(t)
	t.Setenv("KNOWBE4_BASE_URL", api.URL+"/v1")
	t.Setenv("KNOWBE4_API_KEY", token)
	flags := rootFlags{timeout: 10 * time.Second}
	c, err := flags.newClient()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Get(context.Background(), "/agent", nil); err != nil {
		t.Fatalf("configured host: %v", err)
	}
	_, err = c.Get(context.Background(), "/redirect", nil)
	if err == nil || !strings.Contains(err.Error(), "leave the configured KnowBe4 host") {
		t.Fatalf("redirect: want the pin refusal, got %v", err)
	}
	if otherHits.Load() != 0 {
		t.Fatalf("the other host was contacted %d time(s)", otherHits.Load())
	}
}

// The pin refuses every origin but the API base URL's: another host (KnowBe4's
// web app and marketing hosts included), a scheme downgrade, another port and
// a non-ASCII spelling of the host.
func TestPinAllowsOnlyTheAPIOrigin(t *testing.T) {
	pin := normalizedOrigin(&url.URL{Scheme: "https", Host: "us.api.knowbe4.com"})
	for raw, ok := range map[string]bool{
		"https://us.api.knowbe4.com/v1/agent":       true,
		"https://US.API.KnowBe4.com:443/v1/agent":   true,
		"http://us.api.knowbe4.com/v1/agent":        false,
		"https://us.api.knowbe4.com:8443/v1":        false,
		"https://www.knowbe4.com/v1/agent":          false,
		"https://evil.example/v1/agent":             false,
		"https://us.api.knоwbe4.com/v1":             false, // Cyrillic o
		"https://us.api.knowbe4.com.evil.example/x": false,
	} {
		req, _ := http.NewRequest(http.MethodGet, raw, nil)
		if err := checkPin(pin, req); (err == nil) != ok {
			t.Errorf("%s: allowed=%v, want %v (%v)", raw, err == nil, ok, err)
		}
	}
	req, _ := http.NewRequest(http.MethodGet, "https://us.api.knowbe4.com/v1", nil)
	if checkPin("", req) == nil {
		t.Error("an empty pin (unparseable base URL) must refuse everything")
	}
}

// An operator who sets KNOWBE4_BASE_URL (a proxy in front of the API) moves the
// pin with it: that host is the one origin allowed and the default is refused.
func TestPinFollowsTheConfiguredBaseURL(t *testing.T) {
	c := &client.Client{BaseURL: "https://knowbe4-proxy.example.com/v1", HTTPClient: &http.Client{}}
	guardClient(c)
	pin, ok := c.HTTPClient.Transport.(originPinTripper)
	if !ok {
		t.Fatalf("the origin pin is not outermost: %T", c.HTTPClient.Transport)
	}
	if pin.pin != "https://knowbe4-proxy.example.com:443" {
		t.Fatalf("pin = %q, want the configured origin", pin.pin)
	}
	req, _ := http.NewRequest(http.MethodGet, "https://us.api.knowbe4.com/v1/agent", nil)
	if _, err := c.HTTPClient.Do(req); !errors.Is(err, ErrOriginChanged) {
		t.Fatalf("default host under another pin: want ErrOriginChanged, got %v", err)
	}
	unparsed := &client.Client{BaseURL: "https://us.api.knowbe4.com:bad/v1", HTTPClient: &http.Client{}}
	guardClient(unparsed)
	if upin, ok := unparsed.HTTPClient.Transport.(originPinTripper); !ok || upin.pin != "" {
		t.Fatalf("an unparseable base URL must pin nothing, got %T", unparsed.HTTPClient.Transport)
	}
}

// The pin is the outermost transport, the response cache is off, a second
// hook run does not wrap the chain again, and under --dry-run nothing is sent.
func TestGuardWrapsTheTransport(t *testing.T) {
	c := &client.Client{BaseURL: "https://us.api.knowbe4.com/v1", HTTPClient: &http.Client{}}
	guardClient(c)
	pin, ok := c.HTTPClient.Transport.(originPinTripper)
	if !ok {
		t.Fatalf("the origin pin is not outermost: %T", c.HTTPClient.Transport)
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
		"https://US.API.KnowBe4.com/v1":   "https://us.api.knowbe4.com:443",
		"https://us.api.knowbe4.com:8443": "https://us.api.knowbe4.com:8443",
		"http://127.0.0.1/x":              "http://127.0.0.1:80",
		"https://us.api.knоwbe4.com/v1/x": "",
		"https://[::1]:9443/v1":           "https://[::1]:9443",
	} {
		u, _ := url.Parse(raw)
		if got := normalizedOrigin(u); got != want {
			t.Errorf("%s: got %q, want %q", raw, got, want)
		}
	}
}

// The token from every source is held the moment config loads: the
// environment (KNOWBE4_API_KEY), the credential store (auth set-token)
// and a config file's api_key; the environment wins and goes on the wire as
// `Authorization: Bearer <key>`, and a stored token it overrides is held all
// the same.
func TestTokenFromEverySourceIsHeld(t *testing.T) {
	const envTok, storedTok, fileTok = "envUnitToken-2c4e6a8b0d", "storedUnitToken-9f7d5b3a1c", "fileUnitToken-3e5a7c9b1d"
	var got atomic.Value
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.Store(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
		_, _ = w.Write([]byte(`{"data":[],"pagination":{"next":null}}`))
	}))
	t.Cleanup(api.Close)
	testenv.Isolate(t)
	t.Setenv("KNOWBE4_BASE_URL", api.URL+"/v1")
	t.Setenv("KNOWBE4_API_KEY", "")
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
			t.Fatalf("Bearer token on the wire = %v, want %s", got.Load(), what)
		}
	}
	wire(storedTok, "the stored token")
	t.Setenv("KNOWBE4_API_KEY", envTok)
	c, err := flags.newClient()
	if err != nil {
		t.Fatal(err)
	}
	if c.Config.AuthSource != "env:KNOWBE4_API_KEY" {
		t.Errorf("AuthSource = %q, want env:KNOWBE4_API_KEY", c.Config.AuthSource)
	}
	wire(envTok, "KNOWBE4_API_KEY (it wins over the stored token)")
	// A 0600 config file's api_key (KNOWBE4_CONFIG), no environment token.
	cfgPath := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(cfgPath, []byte(fmt.Sprintf("api_key = %q\n", fileTok)), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KNOWBE4_API_KEY", "")
	t.Setenv("KNOWBE4_CONFIG", cfgPath)
	wire(fileTok, "the config file's api_key")
}
