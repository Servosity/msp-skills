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

	"abnormal-pp-cli/internal/client"
	"abnormal-pp-cli/internal/cliutil/testenv"
)

// api-origin-pinned: the Authorization bearer token reaches only the API
// host, a redirect to another host is refused before it is followed, and the
// configured host still answers.
func TestOriginPinKeepsTheTokenOnTheAPIHost(t *testing.T) {
	const token = "pinUnitAbnormalToken-7e3a9c1f5b"
	var otherHits atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		otherHits.Add(1)
		_, _ = w.Write([]byte(`{"threats":[]}`))
	}))
	t.Cleanup(other.Close)
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+token {
			t.Errorf("the API host got Authorization %q", r.Header.Get("Authorization"))
		}
		if strings.HasSuffix(r.URL.Path, "/redirect") {
			http.Redirect(w, r, other.URL+"/v1/threats", http.StatusTemporaryRedirect)
			return
		}
		_, _ = w.Write([]byte(`{"threats":[{"threatId":"184712ab-6d8b-47b3-89d7-a314efef23ff"}],"pageNumber":1}`))
	}))
	t.Cleanup(api.Close)
	testenv.Isolate(t)
	t.Setenv("ABNORMAL_BASE_URL", api.URL+"/v1")
	t.Setenv("ABNORMAL_API_TOKEN", token)
	flags := rootFlags{timeout: 10 * time.Second}
	c, err := flags.newClient()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Get(context.Background(), "/threats", nil); err != nil {
		t.Fatalf("configured host: %v", err)
	}
	_, err = c.Get(context.Background(), "/redirect", nil)
	if err == nil || !strings.Contains(err.Error(), "leave the configured Abnormal host") {
		t.Fatalf("redirect: want the pin refusal, got %v", err)
	}
	if otherHits.Load() != 0 {
		t.Fatalf("the other host was contacted %d time(s)", otherHits.Load())
	}
}

// The pin refuses every origin but the API base URL's: another host (the
// other Abnormal region included), a scheme downgrade, another port and a
// non-ASCII spelling of the host.
func TestPinAllowsOnlyTheAPIOrigin(t *testing.T) {
	pin := normalizedOrigin(&url.URL{Scheme: "https", Host: "api.abnormalplatform.com"})
	for raw, ok := range map[string]bool{
		"https://api.abnormalplatform.com/v1/threats":        true,
		"https://API.AbnormalPlatform.com:443/v1/threats":    true,
		"http://api.abnormalplatform.com/v1/threats":         false,
		"https://api.abnormalplatform.com:8443/v1":           false,
		"https://eu.rest.abnormalsecurity.com/v1/threats":    false,
		"https://evil.example/v1/threats":                    false,
		"https://api.abnormalplatfоrm.com/v1":                false, // Cyrillic o
		"https://api.abnormalplatform.com.evil.example/v1/x": false,
	} {
		req, _ := http.NewRequest(http.MethodGet, raw, nil)
		if err := checkPin(pin, req); (err == nil) != ok {
			t.Errorf("%s: allowed=%v, want %v (%v)", raw, err == nil, ok, err)
		}
	}
	req, _ := http.NewRequest(http.MethodGet, "https://api.abnormalplatform.com/v1", nil)
	if checkPin("", req) == nil {
		t.Error("an empty pin (unparseable base URL) must refuse everything")
	}
}

// An EU tenant sets ABNORMAL_BASE_URL to the EU host: the pin follows the
// configured base URL, so the EU host is the one origin allowed and the US
// default is refused.
func TestPinFollowsTheConfiguredRegion(t *testing.T) {
	c := &client.Client{BaseURL: "https://eu.rest.abnormalsecurity.com/v1", HTTPClient: &http.Client{}}
	guardClient(c)
	pin, ok := c.HTTPClient.Transport.(originPinTripper)
	if !ok {
		t.Fatalf("the origin pin is not outermost: %T", c.HTTPClient.Transport)
	}
	if pin.pin != "https://eu.rest.abnormalsecurity.com:443" {
		t.Fatalf("pin = %q, want the EU origin", pin.pin)
	}
	req, _ := http.NewRequest(http.MethodGet, "https://api.abnormalplatform.com/v1/threats", nil)
	if _, err := c.HTTPClient.Do(req); !errors.Is(err, ErrOriginChanged) {
		t.Fatalf("US host under an EU pin: want ErrOriginChanged, got %v", err)
	}
	unparsed := &client.Client{BaseURL: "https://api.abnormalplatform.com:bad/v1", HTTPClient: &http.Client{}}
	guardClient(unparsed)
	if upin, ok := unparsed.HTTPClient.Transport.(originPinTripper); !ok || upin.pin != "" {
		t.Fatalf("an unparseable base URL must pin nothing, got %T", unparsed.HTTPClient.Transport)
	}
}

// The pin is the outermost transport, the response cache is off, a second
// hook run does not wrap the chain again, and under --dry-run nothing is sent.
// markerTripper stands in for the client's own transport (the credential kit's).
type markerTripper struct{}

func (markerTripper) RoundTrip(*http.Request) (*http.Response, error) { return nil, errors.New("marker") }

func TestGuardWrapsTheTransport(t *testing.T) {
	c := &client.Client{BaseURL: "https://api.abnormalplatform.com/v1", HTTPClient: &http.Client{Transport: markerTripper{}}}
	guardClient(c)
	pin, ok := c.HTTPClient.Transport.(originPinTripper)
	if !ok {
		t.Fatalf("the origin pin is not outermost: %T", c.HTTPClient.Transport)
	}
	if _, ok := pin.next.(markerTripper); !ok {
		t.Fatalf("the pin sends through %T, not the client's own transport", pin.next)
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
		mustRequest(t, http.MethodPost, srv.URL+"/threats/184712ab-6d8b-47b3-89d7-a314efef23ff"),
		mustRequest(t, http.MethodPatch, srv.URL+"/resources/res_42/update"),
		// A read is refused too: a dry run sends nothing at all.
		mustRequest(t, http.MethodGet, srv.URL+"/threats"),
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
		"https://API.AbnormalPlatform.com/v1":   "https://api.abnormalplatform.com:443",
		"https://api.abnormalplatform.com:8443": "https://api.abnormalplatform.com:8443",
		"http://127.0.0.1/x":                    "http://127.0.0.1:80",
		"https://api.abnormalplatfоrm.com/v1/x": "",
		"https://[::1]:9443/v1/threats":         "https://[::1]:9443",
	} {
		u, _ := url.Parse(raw)
		if got := normalizedOrigin(u); got != want {
			t.Errorf("%s: got %q, want %q", raw, got, want)
		}
	}
}
