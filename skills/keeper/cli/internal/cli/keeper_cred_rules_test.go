// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"keeper-pp-cli/internal/client"
	"keeper-pp-cli/internal/cliutil"
	"keeper-pp-cli/internal/cliutil/testenv"
)

// No in-process test runs credkit.Start, so the outbound guard would refuse every
// credentialed request, and these tests talk to plain httptest servers on loopback,
// which the built binaries refuse. internal/keeperblackbox proves both on the
// binaries.
func init() {
	cliutil.OutboundGuard = func(rt http.RoundTripper) http.RoundTripper { return rt }
	loopbackPlainHTTP = true
}

// api-origin-pinned: no redirect is followed (to another host or the same
// one), the api-key never reaches another host, and the configured host still
// answers.
func TestOriginPinRefusesRedirects(t *testing.T) {
	var hits, apiHits atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	t.Cleanup(other.Close)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiHits.Add(1)
		if r.URL.Path != "/health" && r.Header.Get("api-key") != "pinUnitKey-4c1e7a9b" {
			t.Errorf("the configured host got api-key %q", r.Header.Get("api-key"))
		}
		switch r.URL.Path {
		case "/away":
			http.Redirect(w, r, other.URL+"/health", http.StatusTemporaryRedirect)
		case "/same":
			http.Redirect(w, r, "/api/v2/queue/status", http.StatusTemporaryRedirect)
		default:
			_, _ = w.Write([]byte(`{"success":true,"queue_size":0}`))
		}
	}))
	t.Cleanup(srv.Close)
	testenv.Isolate(t)
	t.Setenv("KEEPER_BASE_URL", srv.URL)
	t.Setenv("KEEPER_API_KEY", "pinUnitKey-4c1e7a9b")
	flags := rootFlags{timeout: 10 * time.Second}
	c, err := flags.newClient()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = c.Get(context.Background(), "/away", nil); err == nil || !strings.Contains(err.Error(), "leave the configured Keeper Service Mode host") {
		t.Fatalf("redirect away: want the pin refusal, got %v", err)
	}
	if _, err = c.Get(context.Background(), "/same", nil); err == nil || !strings.Contains(err.Error(), "never follows") {
		t.Fatalf("same-host redirect: want the redirect refusal, got %v", err)
	}
	if _, err = c.Get(context.Background(), "/api/v2/queue/status", nil); err != nil {
		t.Fatalf("configured host: %v", err)
	}
	if hits.Load() != 0 {
		t.Fatalf("the other host was contacted %d time(s)", hits.Load())
	}
	if apiHits.Load() == 0 {
		t.Fatal("the configured host was never asked")
	}
}

// no-plaintext-key: in production a plain http base URL is refused on every host,
// loopback included, before the origin is even compared; https to the pinned origin
// still passes. The built binaries show the same refusal (internal/keeperblackbox).
func TestGuardRefusesPlainHTTPOnEveryHost(t *testing.T) {
	prior := loopbackPlainHTTP
	loopbackPlainHTTP = false
	t.Cleanup(func() { loopbackPlainHTTP = prior })
	plain := []string{
		"http://keeper-svc.example.internal:9090/api/v1/executecommand",
		"http://127.0.0.1:9090/api/v1/executecommand",
		"http://localhost:9090/health",
		"http://[::1]:9090/health",
		"HTTP://127.0.0.1:9090/health",
	}
	origins := pinnedOrigins(plain...)
	for _, raw := range plain {
		u, err := url.Parse(raw)
		if err != nil {
			t.Fatal(err)
		}
		if err := checkPinned(origins, u); !errors.Is(err, ErrPlaintextKey) {
			t.Errorf("%s: got %v, want the plain http refusal", raw, err)
		}
	}
	// The whole tripper refuses too, and never reaches the transport under it.
	var sent atomic.Int32
	tripper := originPinTripper{origins: origins, next: roundTripFunc(func(*http.Request) (*http.Response, error) {
		sent.Add(1)
		return nil, errors.New("reached the transport")
	})}
	req, err := http.NewRequest(http.MethodGet, plain[1], nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tripper.RoundTrip(req); !errors.Is(err, ErrPlaintextKey) {
		t.Errorf("the tripper: got %v, want the plain http refusal", err)
	}
	if sent.Load() != 0 {
		t.Errorf("a plain http request reached the transport under the pin")
	}
	// The control: https to the pinned origin passes, so the refusal is about the scheme.
	secure := "https://127.0.0.1:9090/health"
	u, err := url.Parse(secure)
	if err != nil {
		t.Fatal(err)
	}
	if err := checkPinned(pinnedOrigins(secure), u); err != nil {
		t.Errorf("%s: https to the pinned origin was refused: %v", secure, err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// kitGuardStandIn marks the transport cliutil.OutboundGuard returned.
type kitGuardStandIn struct{ http.RoundTripper }

// The pin is the outermost transport, it sends through the kit's outbound guard,
// the response cache is off, and a second hook run does not wrap the chain again.
func TestGuardWrapsTheTransport(t *testing.T) {
	prior := cliutil.OutboundGuard
	cliutil.OutboundGuard = func(rt http.RoundTripper) http.RoundTripper { return kitGuardStandIn{rt} }
	t.Cleanup(func() { cliutil.OutboundGuard = prior })
	c := &client.Client{BaseURL: "https://localhost:9090", HTTPClient: &http.Client{}}
	guardClient(c)
	pin, ok := c.HTTPClient.Transport.(originPinTripper)
	if !ok {
		t.Fatalf("the origin pin is not outermost: %T", c.HTTPClient.Transport)
	}
	if _, ok := pin.next.(kitGuardStandIn); !ok {
		t.Fatalf("the pin sends through %T, not the kit's outbound guard", pin.next)
	}
	if !c.NoCache {
		t.Fatal("the response cache is on")
	}
	guardClient(c)
	if again, ok := c.HTTPClient.Transport.(originPinTripper); !ok || again.next != pin.next {
		t.Fatal("a second guard run re-wrapped the transport")
	}
}

func TestNormalizedOriginRefusesNonASCIIAndFillsPorts(t *testing.T) {
	for raw, want := range map[string]string{
		"https://Graph.Microsoft.com/v1.0":   "https://graph.microsoft.com:443",
		"https://graph.microsoft.com:8443/x": "https://graph.microsoft.com:8443",
		"http://127.0.0.1/x":                 "http://127.0.0.1:80",
		"https://graph.mİcrosoft.com/v1.0/x": "",
	} {
		u, _ := url.Parse(raw)
		if got := normalizedOrigin(u); got != want {
			t.Errorf("%s: got %q, want %q", raw, got, want)
		}
	}
	got := pinnedOrigins("https://graph.microsoft.com/v1.0", "://bad")
	if len(got) != 1 || !got["https://graph.microsoft.com:443"] {
		t.Errorf("pinned origins: %v", got)
	}
}
