// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gradient-pp-cli/internal/client"
	"gradient-pp-cli/internal/cliutil"
	"gradient-pp-cli/internal/cliutil/testenv"
	"gradient-pp-cli/internal/config"
	"gradient-pp-cli/internal/credkit"
)

// api-origin-pinned: the GRADIENT-TOKEN header reaches only the API host, a
// redirect to another host is refused before it is followed, and the
// configured host still answers.
func TestOriginPinKeepsTheTokenOnTheAPIHost(t *testing.T) {
	const tok = "pinUnitGradientToken-7e3a9c1f5b"
	var otherHits atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		otherHits.Add(1)
		_, _ = w.Write([]byte(`[]`))
	}))
	t.Cleanup(other.Close)
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("GRADIENT-TOKEN") != tok {
			t.Errorf("the API host got GRADIENT-TOKEN %q", r.Header.Get("GRADIENT-TOKEN"))
		}
		if strings.HasSuffix(r.URL.Path, "/redirect") {
			http.Redirect(w, r, other.URL+"/api/vendor-api/organization/accounts", http.StatusTemporaryRedirect)
			return
		}
		_, _ = w.Write([]byte(`[{"id":"a1","name":"Acme","isMapped":true}]`))
	}))
	t.Cleanup(api.Close)
	testenv.Isolate(t)
	t.Setenv("GRADIENT_BASE_URL", api.URL+"/api")
	t.Setenv("GRADIENT_TOKEN", tok)
	flags := rootFlags{timeout: 10 * time.Second}
	c, err := flags.newClient()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Get(context.Background(), "/vendor-api/organization/accounts", nil); err != nil {
		t.Fatalf("configured host: %v", err)
	}
	_, err = c.Get(context.Background(), "/redirect", nil)
	if err == nil || !strings.Contains(err.Error(), "leave the configured Gradient host") {
		t.Fatalf("redirect: want the pin refusal, got %v", err)
	}
	if otherHits.Load() != 0 {
		t.Fatalf("the other host was contacted %d time(s)", otherHits.Load())
	}
}

// The pin refuses every origin but the API base URL's: another host, a scheme
// downgrade, another port and a non-ASCII spelling of the host.
func TestPinAllowsOnlyTheAPIOrigin(t *testing.T) {
	pin := normalizedOrigin(&url.URL{Scheme: "https", Host: "app.usegradient.com"})
	for raw, ok := range map[string]bool{
		"https://app.usegradient.com/api/vendor-api":     true,
		"https://APP.usegradient.com:443/api/vendor-api": true,
		"http://app.usegradient.com/api/vendor-api":      false,
		"https://app.usegradient.com:8443/api":           false,
		"https://evil.example/api/vendor-api":            false,
		"https://app.usegrаdient.com/api":                false, // Cyrillic a
	} {
		req, _ := http.NewRequest(http.MethodGet, raw, nil)
		if err := checkPin(pin, req); (err == nil) != ok {
			t.Errorf("%s: allowed=%v, want %v (%v)", raw, err == nil, ok, err)
		}
	}
	req, _ := http.NewRequest(http.MethodGet, "https://app.usegradient.com/api", nil)
	if checkPin("", req) == nil {
		t.Error("an empty pin (unparseable base URL) must refuse everything")
	}
}

// The pin is the outermost transport and sends through the client's own (the
// credential kit's, in client.New), the response cache is off, a second hook
// run does not wrap the chain again, and under --dry-run nothing is sent.
func TestGuardWrapsTheTransport(t *testing.T) {
	inner := &http.Transport{}
	c := &client.Client{BaseURL: "https://app.usegradient.com/api", HTTPClient: &http.Client{Transport: inner}}
	guardClient(c)
	pin, ok := c.HTTPClient.Transport.(originPinTripper)
	if !ok {
		t.Fatalf("the origin pin is not outermost: %T", c.HTTPClient.Transport)
	}
	if pin.next != http.RoundTripper(inner) {
		t.Fatalf("the pin sends through %T, not the client's transport", pin.next)
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
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/vendor-api/service/svc-1/count", nil)
	if _, err := dry.HTTPClient.Do(req); !errors.Is(err, ErrDryRunSend) {
		t.Fatalf("dry run: want ErrDryRunSend, got %v", err)
	}
	if hits.Load() != 0 {
		t.Fatal("a --dry-run client sent a request")
	}
}

func TestNormalizedOriginRefusesNonASCIIAndFillsPorts(t *testing.T) {
	for raw, want := range map[string]string{
		"https://APP.UseGradient.com/api":       "https://app.usegradient.com:443",
		"https://app.usegradient.com:8443/x":    "https://app.usegradient.com:8443",
		"http://127.0.0.1/x":                    "http://127.0.0.1:80",
		"https://app.usegrаdient.com/api/x":     "",
		"https://[::1]:9443/api/vendor-api/xyz": "https://[::1]:9443",
	} {
		u, _ := url.Parse(raw)
		if got := normalizedOrigin(u); got != want {
			t.Errorf("%s: got %q, want %q", raw, got, want)
		}
	}
}

// composed-auth-key-pair + credkit-wiring: the GRADIENT-TOKEN value derived
// from the key pair is held through the credential kit the moment config
// loads (both keys are held from startup by Declarations.SecretEnv), and the
// derived value is what goes on the wire.
func TestDerivedTokenIsHeldAndSent(t *testing.T) {
	const vendor, partner = "vendorUnitKey-2c4e6a8b", "partnerUnitKey-9f7d5b3a"
	want := base64.StdEncoding.EncodeToString([]byte(vendor + ":" + partner))
	var got atomic.Value
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.Store(r.Header.Get("GRADIENT-TOKEN"))
		_, _ = w.Write([]byte(`{"data":{"name":"Vendor"},"status":200}`))
	}))
	t.Cleanup(api.Close)
	testenv.Isolate(t)
	t.Setenv("GRADIENT_BASE_URL", api.URL+"/api")
	t.Setenv("GRADIENT_TOKEN", "")
	t.Setenv(config.EnvVendorAPIKey, vendor)
	t.Setenv(config.EnvPartnerAPIKey, partner)
	var held []string
	restore := cliutil.Hold
	cliutil.Hold = func(src credkit.Source, vs ...string) {
		if src == "minted:gradient-token" {
			held = append(held, vs...)
		}
	}
	t.Cleanup(func() { cliutil.Hold = restore })
	flags := rootFlags{timeout: 10 * time.Second}
	c, err := flags.newClient()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(held, want) {
		t.Errorf("the derived GRADIENT-TOKEN is not held after config load (held %d values)", len(held))
	}
	if _, err := c.Get(context.Background(), "/vendor-api", nil); err != nil {
		t.Fatal(err)
	}
	if got.Load() != want {
		t.Fatalf("GRADIENT-TOKEN on the wire = %v, want the derived %q", got.Load(), want)
	}
}
