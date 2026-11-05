// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"liongard-pp-cli/internal/client"
	"liongard-pp-cli/internal/cliutil/testenv"
	"liongard-pp-cli/internal/config"
	"liongard-pp-cli/internal/credkit"
)

// No in-process test runs credkit.Start, so the kit's outbound guard would
// refuse every credentialed request to these loopback servers.
// internal/liongardblackbox proves the guard on the built binaries.
func init() {
	client.OutboundGuard = func(rt http.RoundTripper) http.RoundTripper { return rt }
}

// heldNow reports whether the credential kit withholds v from a file.
func heldNow(t *testing.T, v string) bool {
	t.Helper()
	err := credkit.WriteFile(filepath.Join(t.TempDir(), "probe"), []byte(v), 0o600)
	if err != nil && !errors.Is(err, credkit.ErrWithheld) {
		t.Fatal(err)
	}
	return errors.Is(err, credkit.ErrWithheld)
}

// api-origin-pinned: the X-ROAR-API-KEY header reaches only the API host, a
// redirect to another host is refused before it is followed, and the
// configured host still answers.
func TestOriginPinKeepsTheTokenOnTheAPIHost(t *testing.T) {
	const key = "pinUnitLiongardKey-7e3a9c1f5b"
	var otherHits atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		otherHits.Add(1)
		_, _ = w.Write([]byte(`[]`))
	}))
	t.Cleanup(other.Close)
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-ROAR-API-KEY") != key {
			t.Errorf("the API host got X-ROAR-API-KEY %q", r.Header.Get("X-ROAR-API-KEY"))
		}
		if strings.HasSuffix(r.URL.Path, "/redirect") {
			http.Redirect(w, r, other.URL+"/api/v1/environments", http.StatusTemporaryRedirect)
			return
		}
		_, _ = w.Write([]byte(`[{"ID":42,"Name":"Acme","Visible":true}]`))
	}))
	t.Cleanup(api.Close)
	testenv.Isolate(t)
	t.Setenv("LIONGARD_BASE_URL", api.URL+"/api/v1")
	t.Setenv("LIONGARD_API_KEY", key)
	flags := rootFlags{timeout: 10 * time.Second}
	c, err := flags.newClient()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Get(context.Background(), "/environments", nil); err != nil {
		t.Fatalf("configured host: %v", err)
	}
	_, err = c.Get(context.Background(), "/redirect", nil)
	if err == nil || !strings.Contains(err.Error(), "leave the configured Liongard host") {
		t.Fatalf("redirect: want the pin refusal, got %v", err)
	}
	if otherHits.Load() != 0 {
		t.Fatalf("the other host was contacted %d time(s)", otherHits.Load())
	}
}

// The pin refuses every origin but the API base URL's: another host, a scheme
// downgrade, another port, another Liongard instance and a non-ASCII spelling
// of the host.
func TestPinAllowsOnlyTheAPIOrigin(t *testing.T) {
	pin := normalizedOrigin(&url.URL{Scheme: "https", Host: "us1.app.liongard.com"})
	for raw, ok := range map[string]bool{
		"https://us1.app.liongard.com/api/v1/environments":     true,
		"https://US1.app.liongard.com:443/api/v1/environments": true,
		"http://us1.app.liongard.com/api/v1/environments":      false,
		"https://us1.app.liongard.com:8443/api/v1":             false,
		"https://us2.app.liongard.com/api/v1/environments":     false,
		"https://evil.example/api/v1/environments":             false,
		"https://us1.app.liоngard.com/api/v1":                  false, // Cyrillic o
	} {
		req, _ := http.NewRequest(http.MethodGet, raw, nil)
		if err := checkPin(pin, req); (err == nil) != ok {
			t.Errorf("%s: allowed=%v, want %v (%v)", raw, err == nil, ok, err)
		}
	}
	req, _ := http.NewRequest(http.MethodGet, "https://us1.app.liongard.com/api/v1", nil)
	if checkPin("", req) == nil {
		t.Error("an empty pin (unparseable base URL) must refuse everything")
	}
}

// The shipped base URL is per instance (https://{instance}.app.liongard.com/api/v1):
// the pin is the RESOLVED origin, and an unresolved template pins nothing, so
// every request is refused before it is sent.
func TestPinFollowsTheResolvedInstance(t *testing.T) {
	const base = "https://{instance}.app.liongard.com/api/v1"
	c := &client.Client{BaseURL: base, HTTPClient: &http.Client{},
		Config: &config.Config{BaseURL: base, TemplateVars: map[string]string{"instance": "us1"}}}
	guardClient(c)
	pin, ok := c.HTTPClient.Transport.(originPinTripper)
	if !ok {
		t.Fatalf("the origin pin is not outermost: %T", c.HTTPClient.Transport)
	}
	if pin.pin != "https://us1.app.liongard.com:443" {
		t.Fatalf("pin = %q, want the resolved instance origin", pin.pin)
	}
	unresolved := &client.Client{BaseURL: base, HTTPClient: &http.Client{}}
	guardClient(unresolved)
	upin, ok := unresolved.HTTPClient.Transport.(originPinTripper)
	if !ok || upin.pin != "" {
		t.Fatalf("an unresolved template must pin nothing, got %T %q", unresolved.HTTPClient.Transport, upin.pin)
	}
	req, _ := http.NewRequest(http.MethodGet, "https://us1.app.liongard.com/api/v1/environments", nil)
	if _, err := unresolved.HTTPClient.Do(req); !errors.Is(err, ErrOriginChanged) {
		t.Fatalf("unresolved template: want ErrOriginChanged, got %v", err)
	}
}

// The pin is the outermost transport, the response cache is off, a second
// hook run does not wrap the chain again, and under --dry-run nothing is sent.
func TestGuardWrapsTheTransport(t *testing.T) {
	c := &client.Client{BaseURL: "https://us1.app.liongard.com/api/v1", HTTPClient: &http.Client{}}
	guardClient(c)
	pin, ok := c.HTTPClient.Transport.(originPinTripper)
	if !ok {
		t.Fatalf("the origin pin is not outermost: %T", c.HTTPClient.Transport)
	}
	if pin.next != http.DefaultTransport { // the kit's guard is the identity in these tests (init)
		t.Fatalf("the pin sends through %T, not the outbound guard over the client transport", pin.next)
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
		mustRequest(t, http.MethodPost, srv.URL+"/environments"),
		// The GET that starts an inspection run is a write too.
		mustRequest(t, http.MethodGet, srv.URL+"/launchpoints/1203/run"),
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
		"https://US1.App.Liongard.com/api/v1":    "https://us1.app.liongard.com:443",
		"https://us1.app.liongard.com:8443/x":    "https://us1.app.liongard.com:8443",
		"http://127.0.0.1/x":                     "http://127.0.0.1:80",
		"https://us1.app.liоngard.com/api/v1/x":  "",
		"https://[::1]:9443/api/v1/environments": "https://[::1]:9443",
	} {
		u, _ := url.Parse(raw)
		if got := normalizedOrigin(u); got != want {
			t.Errorf("%s: got %q, want %q", raw, got, want)
		}
	}
}

// composed-roar-auth-from-id-and-secret + output-chokepoint: the access key
// ID and secret, the pair and the X-ROAR-API-KEY value derived from it are
// held the moment config loads, and the derived value is what goes on the
// wire.
func TestDerivedKeyIsHeldAndSent(t *testing.T) {
	if inFreshProcess(t) {
		return // the held derived forms stay held until the process exits
	}
	const id, secret = "keyIdUnit-2c4e6a8b", "keySecretUnit-9f7d5b3a"
	want := base64.StdEncoding.EncodeToString([]byte(id + ":" + secret))
	var got atomic.Value
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.Store(r.Header.Get("X-ROAR-API-KEY"))
		_, _ = w.Write([]byte(`[{"ID":42,"Name":"Acme"}]`))
	}))
	t.Cleanup(api.Close)
	testenv.Isolate(t)
	t.Setenv("LIONGARD_BASE_URL", api.URL+"/api/v1")
	t.Setenv("LIONGARD_API_KEY", "")
	t.Setenv("LIONGARD_ACCESS_KEY_ID", id)
	t.Setenv("LIONGARD_ACCESS_KEY_SECRET", secret)
	flags := rootFlags{timeout: 10 * time.Second}
	c, err := flags.newClient()
	if err != nil {
		t.Fatal(err)
	}
	if c.Config.AuthSource != "env:LIONGARD_ACCESS_KEY_ID+SECRET" {
		t.Errorf("AuthSource = %q, want the composed pair", c.Config.AuthSource)
	}
	// The ID and the secret are SecretEnv, held by credkit.Start; the forms
	// derived from them are held where config.Load makes them.
	for _, v := range []string{id + ":" + secret, want} {
		if !heldNow(t, v) {
			t.Errorf("%q is not held after config load", v)
		}
	}
	if _, err := c.Get(context.Background(), "/environments", nil); err != nil {
		t.Fatal(err)
	}
	if got.Load() != want {
		t.Fatalf("X-ROAR-API-KEY on the wire = %v, want the derived %q", got.Load(), want)
	}
}

// LIONGARD_API_KEY wins over the pair: the pre-encoded key goes on the wire,
// not the derived one.
func TestAPIKeyWinsOverThePair(t *testing.T) {
	const key, id, secret = "winsUnitKey-4d6f8a0c", "unusedIdUnit-1b3d5f", "unusedSecretUnit-7a9c1e"
	var got atomic.Value
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.Store(r.Header.Get("X-ROAR-API-KEY"))
		_, _ = w.Write([]byte(`[]`))
	}))
	t.Cleanup(api.Close)
	testenv.Isolate(t)
	t.Setenv("LIONGARD_BASE_URL", api.URL+"/api/v1")
	t.Setenv("LIONGARD_API_KEY", key)
	t.Setenv("LIONGARD_ACCESS_KEY_ID", id)
	t.Setenv("LIONGARD_ACCESS_KEY_SECRET", secret)
	flags := rootFlags{timeout: 10 * time.Second}
	c, err := flags.newClient()
	if err != nil {
		t.Fatal(err)
	}
	if c.Config.AuthSource != "env:LIONGARD_API_KEY" {
		t.Errorf("AuthSource = %q, want env:LIONGARD_API_KEY", c.Config.AuthSource)
	}
	if _, err := c.Get(context.Background(), "/environments", nil); err != nil {
		t.Fatal(err)
	}
	if got.Load() != key {
		t.Fatalf("X-ROAR-API-KEY on the wire = %v, want LIONGARD_API_KEY", got.Load())
	}
}

// The session token (LIONGARD_ENDPOINTS_SESSION_TOKEN_AUTH) is held at config
// load and goes out as X-Auth-Token only on the SessionTokenAuth operation
// (POST /authentication/verify-token), never on an ApiKeyAuth one.
func TestSessionTokenHeldAndSentOnlyOnItsOperation(t *testing.T) {
	const key, session = "sessUnitKey-0e2c4a6b", "sessUnitToken-8d1f3b5c"
	var mu sync.Mutex
	seen := map[string]string{}
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen[r.Method+" "+r.URL.Path] = r.Header.Get("X-Auth-Token")
		mu.Unlock()
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(api.Close)
	testenv.Isolate(t)
	t.Setenv("LIONGARD_BASE_URL", api.URL+"/api/v1")
	t.Setenv("LIONGARD_API_KEY", key)
	t.Setenv("LIONGARD_ENDPOINTS_SESSION_TOKEN_AUTH", session)
	flags := rootFlags{timeout: 10 * time.Second}
	c, err := flags.newClient()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Get(context.Background(), "/environments", nil); err != nil {
		t.Fatal(err)
	}
	if _, _, err := c.Post(context.Background(), "/environments", map[string]any{"Name": "Acme"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := c.Post(context.Background(), "/authentication/verify-token", map[string]any{"Token": "123456"}); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if got := seen["POST /api/v1/authentication/verify-token"]; got != session {
		t.Errorf("verify-token X-Auth-Token = %q, want the session token", got)
	}
	for _, op := range []string{"GET /api/v1/environments", "POST /api/v1/environments"} {
		if got, ok := seen[op]; !ok || got != "" {
			t.Errorf("%s: X-Auth-Token %q (seen %v), want none", op, got, ok)
		}
	}
}
