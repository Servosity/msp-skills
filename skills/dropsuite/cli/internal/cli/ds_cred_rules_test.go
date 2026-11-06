// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"dropsuite-pp-cli/internal/client"
	"dropsuite-pp-cli/internal/cliutil/testenv"
)

const (
	pinUnitAccess   = "pinUnitAccessToken-4c1e7a9b"
	pinUnitReseller = "pinUnitResellerTok-8d2f6a1c"
	pinUnitOrgToken = "pinUnitOrgToken-3e9b5c7d"
)

// api-origin-pinned: a redirect to another host is refused before it is
// sent, for the reseller token and for a routed organization token alike;
// the other host never sees a request, and the configured host still answers.
func TestOriginPinRefusesRedirectToAnotherHost(t *testing.T) {
	var hits, apiHits atomic.Int32
	var leaked atomic.Bool
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		for _, h := range []string{"X-Access-Token", "X-Reseller-Token"} {
			if r.Header.Get(h) != "" {
				leaked.Store(true)
			}
		}
		_, _ = w.Write([]byte(`[]`))
	}))
	t.Cleanup(other.Close)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiHits.Add(1)
		switch r.URL.Path {
		case "/api/users":
			_, _ = w.Write([]byte(`[{"id":"26561","email":"owner@example.com","authentication_token":"` + pinUnitOrgToken + `"}]`))
		case "/api/status", "/api/accounts":
			http.Redirect(w, r, other.URL+r.URL.Path, http.StatusTemporaryRedirect)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	testenv.Isolate(t)
	t.Setenv("DROPSUITE_BASE_URL", srv.URL+"/api")
	t.Setenv("DROPSUITE_ACCESS_TOKEN", pinUnitAccess)
	t.Setenv("DROPSUITE_RESELLER_TOKEN", pinUnitReseller)
	t.Setenv(defaultOrgEnv, "26561")
	flags := rootFlags{}
	c, err := flags.newClient()
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/status", "/accounts"} {
		_, err = c.Get(context.Background(), path, nil)
		if err == nil || !strings.Contains(err.Error(), "leave the configured Dropsuite API host") {
			t.Fatalf("%s redirect: want the pin refusal, got %v", path, err)
		}
	}
	if hits.Load() != 0 || leaked.Load() {
		t.Fatalf("the other host was contacted %d time(s) (credential sent: %v)", hits.Load(), leaked.Load())
	}
	if apiHits.Load() < 3 {
		t.Fatalf("the configured host answered %d request(s), want the two redirects and the organization list", apiHits.Load())
	}
}

// The pin is the innermost dropsuite transport: the organization-token
// routing and the response redaction both send through it, the response cache
// is off, and a second hook run does not wrap the chain again.
func TestOriginPinUnderEveryDropsuiteTransport(t *testing.T) {
	testenv.Isolate(t)
	t.Setenv("DROPSUITE_HOST", "euw.dropmyemail.com")
	flags := rootFlags{}
	c, err := flags.newClient()
	if err != nil {
		t.Fatal(err)
	}
	routing, ok := c.HTTPClient.Transport.(orgRoutingTransport)
	if !ok {
		t.Fatalf("outermost transport is %T, want the organization routing", c.HTTPClient.Transport)
	}
	redact, ok := routing.next.(redactingTransport)
	if !ok {
		t.Fatalf("routing sends through %T, want the redaction transport", routing.next)
	}
	pin, ok := redact.next.(originPinTripper)
	if !ok {
		t.Fatalf("redaction sends through %T, not the origin pin", redact.next)
	}
	if len(pin.origins) != 1 || !pin.origins["https://euw.dropmyemail.com:443"] {
		t.Fatalf("pinned origins %v, want the partner host from DROPSUITE_HOST", pin.origins)
	}
	if !c.NoCache {
		t.Fatal("the response cache is on")
	}
	if err := ApplyClientHooks(c); err != nil {
		t.Fatal(err)
	}
	r2, ok := c.HTTPClient.Transport.(orgRoutingTransport)
	if !ok {
		t.Fatal("a second hook run replaced the routing transport")
	}
	if rd, ok := r2.next.(redactingTransport); !ok {
		t.Fatal("a second hook run re-wrapped the transport chain")
	} else if _, ok := rd.next.(originPinTripper); !ok {
		t.Fatal("a second hook run re-wrapped the transport chain")
	}
}

func TestNormalizedOriginRefusesNonASCIIAndFillsPorts(t *testing.T) {
	for raw, want := range map[string]string{
		"https://Dropsuite.US/api":      "https://dropsuite.us:443",
		"https://dropsuite.us:8443/api": "https://dropsuite.us:8443",
		"http://127.0.0.1/api":          "http://127.0.0.1:80",
		"https://İ.dropsuite.us/api":    "",
	} {
		u, _ := url.Parse(raw)
		if got := normalizedOrigin(u); got != want {
			t.Errorf("%s: got %q, want %q", raw, got, want)
		}
	}
	got := pinnedOrigins("https://dropsuite.us/api", "://bad")
	if len(got) != 1 || !got["https://dropsuite.us:443"] {
		t.Errorf("pinned origins: %v", got)
	}
	got = pinnedOrigins(client.ResolvedBaseURL("https://{host}/api", map[string]string{"host": "us.office.hycu.com"}))
	if len(got) != 1 || !got["https://us.office.hycu.com:443"] {
		t.Errorf("templated base: pinned origins %v", got)
	}
	// An unresolved template pins nothing: every request is refused.
	if got := pinnedOrigins(client.ResolvedBaseURL("https://{host}/api", nil)); len(got) != 0 {
		t.Errorf("unresolved template pinned %v", got)
	}
}
