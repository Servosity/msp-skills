// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"compress/zlib"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"exchange-online-pp-cli/internal/client"
	"exchange-online-pp-cli/internal/cliutil"
	"exchange-online-pp-cli/internal/cliutil/testenv"
	"exchange-online-pp-cli/internal/credkit"
)

// No in-process test runs credkit.Start, so the outbound guard would refuse every
// credentialed request, and a value held here would stay held for the rest of the
// test process. internal/exoblackbox proves both on the built binaries.
func init() {
	cliutil.OutboundGuard = func(rt http.RoundTripper) http.RoundTripper { return rt }
	cliutil.Hold = func(credkit.Source, ...string) {}
}

// recordHolds swaps the hold seam for a recorder for one test. Tests that use it
// do not run in parallel.
func recordHolds(t *testing.T) *[]string {
	t.Helper()
	var got []string
	prior := cliutil.Hold
	cliutil.Hold = func(_ credkit.Source, values ...string) { got = append(got, values...) }
	t.Cleanup(func() { cliutil.Hold = prior })
	return &got
}

// api-origin-pinned: neither a redirect nor an @odata.nextLink to another host
// reaches it, and the bearer token is never sent there; the configured API
// host still answers the first page.
func TestOriginPinRefusesRedirectToAnotherHost(t *testing.T) {
	const tenant = "0550b473-9fd2-4dbb-a058-a1640b4bf458"
	var hits, apiHits atomic.Int32
	// The generated client speaks HTTP/2 only: TLS + h2 fakes, --insecure.
	other := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		_, _ = w.Write([]byte(`{"value":[]}`))
	}))
	other.EnableHTTP2 = true
	other.StartTLS()
	t.Cleanup(other.Close)
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiHits.Add(1)
		if r.Header.Get("Authorization") != "Bearer pinUnitToken-4c1e7a9b" {
			t.Errorf("the configured host got Authorization %q", r.Header.Get("Authorization"))
		}
		if strings.HasSuffix(r.URL.Path, "/redirect/InvokeCommand") {
			http.Redirect(w, r, other.URL+"/adminapi/beta/"+tenant+"/InvokeCommand", http.StatusTemporaryRedirect)
			return
		}
		_, _ = w.Write([]byte(`{"value":[{"Name":"a"}],"@odata.nextLink":"` + other.URL + `/adminapi/beta/` + tenant + `/InvokeCommand?$skiptoken=2"}`))
	}))
	srv.EnableHTTP2 = true
	srv.StartTLS()
	t.Cleanup(srv.Close)
	testenv.Isolate(t)
	t.Setenv("EXCHANGE_ONLINE_BASE_URL", srv.URL)
	t.Setenv("EXCHANGE_ONLINE_TOKEN", "pinUnitToken-4c1e7a9b")
	flags := rootFlags{insecure: true}
	c, err := flags.newClient()
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = c.Post(context.Background(), "/adminapi/beta/redirect/InvokeCommand#Get-Mailbox", map[string]any{})
	if err == nil || !strings.Contains(err.Error(), "leave the configured Exchange Online API host") {
		t.Fatalf("redirect: want the pin refusal, got %v", err)
	}
	_, _, err = c.Post(context.Background(), "/adminapi/beta/"+tenant+"/InvokeCommand#Get-Mailbox", map[string]any{})
	if err == nil || !strings.Contains(err.Error(), "another origin") {
		t.Fatalf("nextLink: want the origin refusal, got %v", err)
	}
	if hits.Load() != 0 {
		t.Fatalf("the other host was contacted %d time(s)", hits.Load())
	}
	if apiHits.Load() == 0 {
		t.Fatal("the configured host was never asked")
	}
}

// An unguarded origin in the transport chain is the class this pin closes: a
// request the cmdlet transport builds for any path still passes the pin.
func TestOriginPinCoversEveryCmdletPage(t *testing.T) {
	c := &client.Client{BaseURL: "https://outlook.office365.com", HTTPClient: &http.Client{}}
	guardClient(c)
	if err := installEXOTransport(c); err != nil {
		t.Fatal(err)
	}
	exo, ok := c.HTTPClient.Transport.(*exoTransport)
	if !ok {
		t.Fatalf("the cmdlet transport is not outermost: %T", c.HTTPClient.Transport)
	}
	if _, ok := exo.base.(originPinTripper); !ok {
		t.Fatalf("the cmdlet transport sends through %T, not the origin pin", exo.base)
	}
	if !c.NoCache {
		t.Fatal("the response cache is on")
	}
	guardClient(c) // a second hook run must not wrap the chain again
	if _, ok := c.HTTPClient.Transport.(*exoTransport); !ok {
		t.Fatal("a second guard run re-wrapped the transport")
	}
}

func TestNormalizedOriginRefusesNonASCIIAndFillsPorts(t *testing.T) {
	for raw, want := range map[string]string{
		"https://Outlook.Office365.com/adminapi": "https://outlook.office365.com:443",
		"https://outlook.office365.com:8443/x":   "https://outlook.office365.com:8443",
		"http://127.0.0.1/x":                     "http://127.0.0.1:80",
		"https://İ.office365.com/x":              "",
	} {
		u, _ := url.Parse(raw)
		if got := normalizedOrigin(u); got != want {
			t.Errorf("%s: got %q, want %q", raw, got, want)
		}
	}
	got := pinnedOrigins("https://outlook.office365.com", "://bad")
	if len(got) != 1 || !got["https://outlook.office365.com:443"] {
		t.Errorf("pinned origins: %v", got)
	}
}

// response-credentials-held: a credential a response carries is held before any
// command reads the body and replaced by name, deflate-encoded or not, at any depth;
// masks and names that are not credentials are neither.
func TestResponseCredentialsHeldAndReplacedByName(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			var z bytes.Buffer
			zw := zlib.NewWriter(&z)
			_, _ = zw.Write([]byte(`{"id":"k1","name":"getNameNotSecretA7z1","clientSecret":"mintedSecretUnitDeflate88"}`))
			_ = zw.Close()
			w.Header().Set("Content-Encoding", "deflate")
			_, _ = w.Write(z.Bytes())
			return
		}
		_, _ = w.Write([]byte(`{"keyId":"AKIAUNIT","secretAccessKey":"s3SecretUnitF6g7h8i9j0","nested":{"psk":"tunnelPskUnitA1b2c3"},"password":"**********","name":"getNameNotSecretK2m4"}`))
	}))
	t.Cleanup(srv.Close)
	got := recordHolds(t)
	rt := responseHoldTripper{next: http.DefaultTransport}
	bodies := ""
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		req, _ := http.NewRequest(method, srv.URL+"/adminapi/beta/t/InvokeCommand", nil)
		resp, err := rt.RoundTrip(req)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		bodies += string(body)
	}
	for _, v := range []string{"s3SecretUnitF6g7h8i9j0", "tunnelPskUnitA1b2c3", "mintedSecretUnitDeflate88"} {
		if !slices.Contains(*got, v) {
			t.Errorf("%s not held: %q", v, *got)
		}
		if strings.Contains(bodies, v) {
			t.Errorf("%s not replaced by name:\n%s", v, bodies)
		}
	}
	for _, v := range []string{"getNameNotSecretK2m4", "getNameNotSecretA7z1", "**********", "AKIAUNIT", "k1"} {
		if slices.Contains(*got, v) {
			t.Errorf("%s held, but it is not a credential", v)
		}
	}
	for _, v := range []string{"getNameNotSecretK2m4", "getNameNotSecretA7z1", "[redacted]"} {
		if !strings.Contains(bodies, v) {
			t.Errorf("the body lost %s:\n%s", v, bodies)
		}
	}
}
