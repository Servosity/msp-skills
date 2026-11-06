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

	"autotask-pp-cli/internal/client"
	"autotask-pp-cli/internal/cliutil/testenv"
)

// api-origin-pinned: the ApiIntegrationCode header reaches only the API host, a
// redirect to another host is refused before it is followed, and the
// configured host still answers.
func TestOriginPinKeepsTheTokenOnTheAPIHost(t *testing.T) {
	const token = "pinUnitAutotaskToken-7e3a9c1f5b"
	var otherHits atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		otherHits.Add(1)
		_, _ = w.Write([]byte(`{"items":[]}`))
	}))
	t.Cleanup(other.Close)
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("ApiIntegrationCode") != token {
			t.Errorf("the API host got ApiIntegrationCode %q", r.Header.Get("ApiIntegrationCode"))
		}
		if r.Header.Get("Authorization") != "" {
			t.Errorf("the API host got an Authorization header %q", r.Header.Get("Authorization"))
		}
		if strings.HasSuffix(r.URL.Path, "/redirect") {
			http.Redirect(w, r, other.URL+"/api/v3/agents", http.StatusTemporaryRedirect)
			return
		}
		_, _ = w.Write([]byte(`{"items":[{"AgentID":1187,"MachineName":"FRONTDESK-01"}],"totalItemCount":1,"page":1,"itemsInPage":20,"totalPages":1}`))
	}))
	t.Cleanup(api.Close)
	testenv.Isolate(t)
	t.Setenv("AUTOTASK_BASE_URL", api.URL+"/api/v3")
	t.Setenv("AUTOTASK_API_INTEGRATION_CODE", token)
	flags := rootFlags{timeout: 10 * time.Second}
	c, err := flags.newClient()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Get(context.Background(), "/agents", nil); err != nil {
		t.Fatalf("configured host: %v", err)
	}
	_, err = c.Get(context.Background(), "/redirect", nil)
	if err == nil || !strings.Contains(err.Error(), "leave the configured Autotask host") {
		t.Fatalf("redirect: want the pin refusal, got %v", err)
	}
	if otherHits.Load() != 0 {
		t.Fatalf("the other host was contacted %d time(s)", otherHits.Load())
	}
}

// The pin refuses every origin but the API base URL's: another host (Autotask's
// web app and marketing hosts included), a scheme downgrade, another port and
// a non-ASCII spelling of the host.
func TestPinAllowsOnlyTheAPIOrigin(t *testing.T) {
	pin := normalizedOrigin(&url.URL{Scheme: "https", Host: "webservices5.autotask.net"})
	for raw, ok := range map[string]bool{
		"https://webservices5.autotask.net/atservicesrest/V1.0/Companies/query":   true,
		"https://WEBSERVICES5.Autotask.net:443/atservicesrest/V1.0/Tickets/query": true,
		"http://webservices5.autotask.net/atservicesrest/V1.0/Companies/query":    false,
		"https://webservices5.autotask.net:8443/atservicesrest/V1.0":              false,
		"https://webservices14.autotask.net/atservicesrest/V1.0/Companies/query":  false, // another zone
		"https://webservices.autotask.net/atservicesrest/V1.0/zoneInformation":    false, // the discovery host
		"https://ww5.autotask.net/Autotask/AutotaskExtend/ExecuteCommand.aspx":    false, // the web app
		"https://evil.example/atservicesrest/V1.0":                                false,
		"https://webservices5.autotаsk.net/atservicesrest/V1.0":                   false, // Cyrillic a
		"https://webservices5.autotask.net.evil.example/x":                        false,
	} {
		req, _ := http.NewRequest(http.MethodGet, raw, nil)
		if err := checkPin(pin, req); (err == nil) != ok {
			t.Errorf("%s: allowed=%v, want %v (%v)", raw, err == nil, ok, err)
		}
	}
	req, _ := http.NewRequest(http.MethodGet, "https://webservices5.autotask.net/atservicesrest/V1.0", nil)
	if checkPin("", req) == nil {
		t.Error("an empty pin (unparseable base URL) must refuse everything")
	}
}

// An operator who sets AUTOTASK_BASE_URL (a proxy in front of the API) moves the
// pin with it: that host is the one origin allowed and the default is refused.
func TestPinFollowsTheConfiguredBaseURL(t *testing.T) {
	c := &client.Client{BaseURL: "https://autotask-proxy.example.com/api/v3", HTTPClient: &http.Client{}}
	guardClient(c)
	pin, ok := c.HTTPClient.Transport.(originPinTripper)
	if !ok {
		t.Fatalf("the origin pin is not outermost: %T", c.HTTPClient.Transport)
	}
	if pin.pin != "https://autotask-proxy.example.com:443" {
		t.Fatalf("pin = %q, want the configured origin", pin.pin)
	}
	req, _ := http.NewRequest(http.MethodGet, "https://webservices5.autotask.net/atservicesrest/V1.0/agents", nil)
	if _, err := c.HTTPClient.Do(req); !errors.Is(err, ErrOriginChanged) {
		t.Fatalf("default host under another pin: want ErrOriginChanged, got %v", err)
	}
	unparsed := &client.Client{BaseURL: "https://webservices5.autotask.net:bad/api/v3", HTTPClient: &http.Client{}}
	guardClient(unparsed)
	if upin, ok := unparsed.HTTPClient.Transport.(originPinTripper); !ok || upin.pin != "" {
		t.Fatalf("an unparseable base URL must pin nothing, got %T", unparsed.HTTPClient.Transport)
	}
}

// The pin is the outermost transport around the one the client already carries
// (the credential kit's, in a built binary), the response cache is off, a second
// hook run does not wrap the chain again, and under --dry-run nothing is sent.
func TestGuardWrapsTheTransport(t *testing.T) {
	carried := &http.Transport{}
	c := &client.Client{BaseURL: "https://webservices5.autotask.net/atservicesrest/V1.0", HTTPClient: &http.Client{Transport: carried}}
	guardClient(c)
	pin, ok := c.HTTPClient.Transport.(originPinTripper)
	if !ok {
		t.Fatalf("the origin pin is not outermost: %T", c.HTTPClient.Transport)
	}
	if pin.next != http.RoundTripper(carried) {
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
		mustRequest(t, http.MethodPost, srv.URL+"/tickets"),
		mustRequest(t, http.MethodPut, srv.URL+"/alerts/90321"),
		mustRequest(t, http.MethodDelete, srv.URL+"/agents/1187"),
		// A read is refused too: a dry run sends nothing at all.
		mustRequest(t, http.MethodGet, srv.URL+"/agents"),
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
		"https://WEBSERVICES5.Autotask.net/atservicesrest/V1.0": "https://webservices5.autotask.net:443",
		"https://webservices5.autotask.net:8443":                "https://webservices5.autotask.net:8443",
		"http://127.0.0.1/x":                                    "http://127.0.0.1:80",
		"https://webservices5.autotаsk.net/atservicesrest/x":    "",
		"https://[::1]:9443/atservicesrest/V1.0":                "https://[::1]:9443",
	} {
		u, _ := url.Parse(raw)
		if got := normalizedOrigin(u); got != want {
			t.Errorf("%s: got %q, want %q", raw, got, want)
		}
	}
}
