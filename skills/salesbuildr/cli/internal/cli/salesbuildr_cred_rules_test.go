// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"salesbuildr-pp-cli/internal/client"
	"salesbuildr-pp-cli/internal/cliutil"
	"salesbuildr-pp-cli/internal/cliutil/testenv"
	"salesbuildr-pp-cli/internal/config"
	"salesbuildr-pp-cli/internal/credkit"
)

// api-origin-pinned: the api-key header reaches only the API host, a
// redirect to another host is refused before it is followed, and the
// configured host still answers.
func TestOriginPinKeepsTheTokenOnTheAPIHost(t *testing.T) {
	const tok = "pinUnitSalesbuildrApiKey-7e3a9c1f5b"
	var otherHits atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		otherHits.Add(1)
		_, _ = w.Write([]byte(`[]`))
	}))
	t.Cleanup(other.Close)
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("api-key") != tok {
			t.Errorf("the API host got api-key %q", r.Header.Get("api-key"))
		}
		if strings.HasSuffix(r.URL.Path, "/redirect") {
			http.Redirect(w, r, other.URL+"/api/vendor-api/organization/accounts", http.StatusTemporaryRedirect)
			return
		}
		_, _ = w.Write([]byte(`[{"id":"a1","name":"Acme","isMapped":true}]`))
	}))
	t.Cleanup(api.Close)
	testenv.Isolate(t)
	t.Setenv("SALESBUILDR_BASE_URL", api.URL+"/api")
	t.Setenv("SALESBUILDR_API_KEY", tok)
	flags := rootFlags{timeout: 10 * time.Second}
	c, err := flags.newClient()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Get(context.Background(), "/vendor-api/organization/accounts", nil); err != nil {
		t.Fatalf("configured host: %v", err)
	}
	_, err = c.Get(context.Background(), "/redirect", nil)
	if err == nil || !strings.Contains(err.Error(), "leave the configured Salesbuildr host") {
		t.Fatalf("redirect: want the pin refusal, got %v", err)
	}
	if otherHits.Load() != 0 {
		t.Fatalf("the other host was contacted %d time(s)", otherHits.Load())
	}
}

// The pin refuses every origin but the API base URL's: another host, a scheme
// downgrade, another port and a non-ASCII spelling of the host.
func TestPinAllowsOnlyTheAPIOrigin(t *testing.T) {
	pin := normalizedOrigin(&url.URL{Scheme: "https", Host: "acme.salesbuildr.com"})
	for raw, ok := range map[string]bool{
		"https://acme.salesbuildr.com/public-api/company":     true,
		"https://ACME.Salesbuildr.com:443/public-api/company": true,
		"http://acme.salesbuildr.com/public-api/company":      false,
		"https://acme.salesbuildr.com:8443/public-api":        false,
		"https://other.salesbuildr.com/public-api/company":    false,
		"https://evil.example/public-api/company":             false,
		"https://аcme.salesbuildr.com/public-api":             false, // Cyrillic a
	} {
		req, _ := http.NewRequest(http.MethodGet, raw, nil)
		if err := checkPin(pin, req); (err == nil) != ok {
			t.Errorf("%s: allowed=%v, want %v (%v)", raw, err == nil, ok, err)
		}
	}
	req, _ := http.NewRequest(http.MethodGet, "https://acme.salesbuildr.com/public-api", nil)
	if checkPin("", req) == nil {
		t.Error("an empty pin (unparseable base URL) must refuse everything")
	}
}

// tenant-is-one-label: the shipped base URL is a tenant template. The pin is
// the RESOLVED host (<tenant>.salesbuildr.com), and a tenant that is not one
// DNS label (it would splice another host into the URL) or is still the
// shipped placeholder refuses every request before anything is sent.
func TestTenantTemplatePinsTheResolvedHost(t *testing.T) {
	const tmpl = "https://{tenant}.salesbuildr.com/public-api"
	for tenant, want := range map[string]string{
		"acme":    "https://acme.salesbuildr.com:443",
		"Acme-IT": "https://acme-it.salesbuildr.com:443",
	} {
		c := &client.Client{BaseURL: tmpl, Config: &config.Config{TemplateVars: map[string]string{"tenant": tenant}}, HTTPClient: &http.Client{}}
		pin, err := pinnedOrigin(c)
		if err != nil || pin != want {
			t.Errorf("tenant %q: pin %q (%v), want %q", tenant, pin, err, want)
		}
	}
	for _, tenant := range []string{"", "your-tenant", "evil.example#", "evil.example/x?", "evil.example", "u@evil.example", "evil:8443",
		"-acme", "acme-", "ac me", "аcme", strings.Repeat("a", 64)} {
		c := &client.Client{BaseURL: tmpl, Config: &config.Config{TemplateVars: map[string]string{"tenant": tenant}}, HTTPClient: &http.Client{}}
		if _, err := pinnedOrigin(c); !errors.Is(err, ErrTenantInvalid) {
			t.Errorf("tenant %q: want ErrTenantInvalid, got %v", tenant, err)
		}
	}
	// End to end: with a valid tenant the resolved host is reached (the
	// generated template must not pin nothing and refuse every request).
	var reached atomic.Value
	ok := &client.Client{BaseURL: tmpl, Config: &config.Config{TemplateVars: map[string]string{"tenant": "acme"}},
		HTTPClient: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			reached.Store(r.URL.Host)
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("[]")), Header: http.Header{}, Request: r}, nil
		})}}
	guardClient(ok)
	okReq, _ := http.NewRequest(http.MethodGet, "https://acme.salesbuildr.com/public-api/company", nil)
	if resp, err := ok.HTTPClient.Do(okReq); err != nil {
		t.Fatalf("tenant acme: the resolved host was refused: %v", err)
	} else {
		_ = resp.Body.Close()
	}
	if reached.Load() != "acme.salesbuildr.com" {
		t.Fatalf("tenant acme: reached %v", reached.Load())
	}
	// End to end: a spliced host is never dialled, with or without a redirect.
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1) }))
	t.Cleanup(srv.Close)
	host := strings.TrimPrefix(srv.URL, "http://")
	c := &client.Client{BaseURL: "http://{tenant}.salesbuildr.com/public-api", Config: &config.Config{TemplateVars: map[string]string{"tenant": host + "#"}}, HTTPClient: &http.Client{}}
	guardClient(c)
	req, _ := http.NewRequest(http.MethodGet, "http://"+host+"#.salesbuildr.com/public-api/company", nil)
	req.Header.Set("api-key", "sbTenantSplice-6f2a")
	if _, err := c.HTTPClient.Do(req); !errors.Is(err, ErrTenantInvalid) {
		t.Fatalf("spliced tenant: want ErrTenantInvalid, got %v", err)
	}
	if hits.Load() != 0 {
		t.Fatal("a spliced tenant host was contacted")
	}
}

// The pin is the outermost transport and sends through the client's own (the
// credential kit's, in client.New), the response cache is off, a second hook
// run does not wrap the chain again, and under --dry-run nothing is sent.
func TestGuardWrapsTheTransport(t *testing.T) {
	inner := &http.Transport{}
	c := &client.Client{BaseURL: "https://acme.salesbuildr.com/public-api", HTTPClient: &http.Client{Transport: inner}}
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
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/company", nil)
	if _, err := dry.HTTPClient.Do(req); !errors.Is(err, ErrDryRunSend) {
		t.Fatalf("dry run: want ErrDryRunSend, got %v", err)
	}
	if hits.Load() != 0 {
		t.Fatal("a --dry-run client sent a request")
	}
}

func TestNormalizedOriginRefusesNonASCIIAndFillsPorts(t *testing.T) {
	for raw, want := range map[string]string{
		"https://ACME.Salesbuildr.com/public-api": "https://acme.salesbuildr.com:443",
		"https://acme.salesbuildr.com:8443/x":     "https://acme.salesbuildr.com:8443",
		"http://127.0.0.1/x":                      "http://127.0.0.1:80",
		"https://аcme.salesbuildr.com/public-api": "",
		"https://[::1]:9443/public-api/company":   "https://[::1]:9443",
	} {
		u, _ := url.Parse(raw)
		if got := normalizedOrigin(u); got != want {
			t.Errorf("%s: got %q, want %q", raw, got, want)
		}
	}
}

// credkit-wiring: the API key from the environment (held from startup by
// Declarations.SecretEnv) is held through the credential kit as the auth
// header the client builds, and it is what goes on the wire, raw in the
// api-key header.
func TestEnvAPIKeyIsHeldAndSent(t *testing.T) {
	const key = "sbUnitApiKey-2c4e6a8b9f7d5b3a"
	var got atomic.Value
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.Store(r.Header.Get("api-key"))
		_, _ = w.Write([]byte(`[{"id":"c1","name":"Category"}]`))
	}))
	t.Cleanup(api.Close)
	testenv.Isolate(t)
	t.Setenv("SALESBUILDR_BASE_URL", api.URL+"/public-api")
	t.Setenv("SALESBUILDR_API_KEY", key)
	var held []string
	restore := cliutil.Hold
	cliutil.Hold = func(src credkit.Source, vs ...string) {
		if src == "minted:authorization" {
			held = append(held, vs...)
		}
	}
	t.Cleanup(func() { cliutil.Hold = restore })
	flags := rootFlags{timeout: 10 * time.Second}
	c, err := flags.newClient()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Get(context.Background(), "/category", nil); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(held, key) {
		t.Errorf("the api-key value is not held before it is sent (held %d values)", len(held))
	}
	if got.Load() != key {
		t.Fatalf("api-key on the wire = %v, want the raw key", got.Load())
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
