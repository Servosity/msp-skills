// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"encoding/base64"
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

	"connectwise-control-pp-cli/internal/client"
	"connectwise-control-pp-cli/internal/cliutil/testenv"
)

// api-origin-pinned: the Basic credential reaches only the instance host, a
// redirect to another host is refused before it is followed, and the
// configured host still answers.
func TestOriginPinKeepsTheTokenOnTheAPIHost(t *testing.T) {
	const user, pass = "pinUnitCwcUser-3d8b1f", "pinUnitCwcToken-7e3a9c1f5b"
	basic := "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+pass))
	var otherHits atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		otherHits.Add(1)
		_, _ = w.Write([]byte(`[]`))
	}))
	t.Cleanup(other.Close)
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != basic {
			t.Errorf("the instance host got Authorization %q", r.Header.Get("Authorization"))
		}
		if strings.HasSuffix(r.URL.Path, "/redirect") {
			http.Redirect(w, r, other.URL+"/Services/SessionGroupService.ashx/session-groups", http.StatusTemporaryRedirect)
			return
		}
		_, _ = w.Write([]byte(`[{"name":"All Machines","sessionType":"Access","sessionCount":12}]`))
	}))
	t.Cleanup(api.Close)
	testenv.Isolate(t)
	t.Setenv("CONNECTWISE_CONTROL_BASE_URL", api.URL)
	t.Setenv("CONNECTWISE_CONTROL_USERNAME", user)
	t.Setenv("CONNECTWISE_CONTROL_PASSWORD", pass)
	flags := rootFlags{timeout: 10 * time.Second}
	c, err := flags.newClient()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Get(context.Background(), "/Services/SessionGroupService.ashx/session-groups", nil); err != nil {
		t.Fatalf("configured host: %v", err)
	}
	_, err = c.Get(context.Background(), "/redirect", nil)
	if err == nil || !strings.Contains(err.Error(), "leave the configured ConnectWise Control host") {
		t.Fatalf("redirect: want the pin refusal, got %v", err)
	}
	if otherHits.Load() != 0 {
		t.Fatalf("the other host was contacted %d time(s)", otherHits.Load())
	}
}

// The pin refuses every origin but the API base URL's: another host (ConnectWise Control's
// web app and marketing hosts included), a scheme downgrade, another port and
// a non-ASCII spelling of the host.
func TestPinAllowsOnlyTheAPIOrigin(t *testing.T) {
	pin := normalizedOrigin(&url.URL{Scheme: "https", Host: "company.screenconnect.com"})
	for raw, ok := range map[string]bool{
		"https://company.screenconnect.com/Companies/query":                       true,
		"https://COMPANY.ScreenConnect.com:443/atservicesrest/V1.0/Tickets/query": true,
		"http://company.screenconnect.com/Companies/query":                        false,
		"https://company.screenconnect.com:8443/atservicesrest/V1.0":              false,
		"https://other.screenconnect.com/Companies/query":                         false, // another zone
		"https://zone.screenconnect.com/zoneInformation":                          false, // the discovery host
		"https://help.screenconnect.com/Guest":                                    false, // the web app
		"https://evil.example/atservicesrest/V1.0":                                false,
		"https://webservices5.autotаsk.net/atservicesrest/V1.0":                   false, // Cyrillic a
		"https://company.screenconnect.com.evil.example/x":                        false,
	} {
		req, _ := http.NewRequest(http.MethodGet, raw, nil)
		if err := checkPin(pin, req); (err == nil) != ok {
			t.Errorf("%s: allowed=%v, want %v (%v)", raw, err == nil, ok, err)
		}
	}
	req, _ := http.NewRequest(http.MethodGet, "https://company.screenconnect.com", nil)
	if checkPin("", req) == nil {
		t.Error("an empty pin (unparseable base URL) must refuse everything")
	}
}

// An operator who sets CONNECTWISE_CONTROL_BASE_URL (a proxy in front of the API) moves the
// pin with it: that host is the one origin allowed and the default is refused.
func TestPinFollowsTheConfiguredBaseURL(t *testing.T) {
	c := &client.Client{BaseURL: "https://cwc-proxy.example.com/api/v3", HTTPClient: &http.Client{}}
	guardClient(c)
	pin, ok := c.HTTPClient.Transport.(originPinTripper)
	if !ok {
		t.Fatalf("the origin pin is not outermost: %T", c.HTTPClient.Transport)
	}
	if pin.pin != "https://cwc-proxy.example.com:443" {
		t.Fatalf("pin = %q, want the configured origin", pin.pin)
	}
	req, _ := http.NewRequest(http.MethodGet, "https://company.screenconnect.com/agents", nil)
	if _, err := c.HTTPClient.Do(req); !errors.Is(err, ErrOriginChanged) {
		t.Fatalf("default host under another pin: want ErrOriginChanged, got %v", err)
	}
	unparsed := &client.Client{BaseURL: "https://company.screenconnect.com:bad/api/v3", HTTPClient: &http.Client{}}
	guardClient(unparsed)
	if upin, ok := unparsed.HTTPClient.Transport.(originPinTripper); !ok || upin.pin != "" {
		t.Fatalf("an unparseable base URL must pin nothing, got %T", unparsed.HTTPClient.Transport)
	}
}

// The pin is the outermost transport, the response cache is off, a second
// hook run does not wrap the chain again, and under --dry-run nothing is sent.
func TestGuardWrapsTheTransport(t *testing.T) {
	c := &client.Client{BaseURL: "https://company.screenconnect.com", HTTPClient: &http.Client{}}
	guardClient(c)
	pin, ok := c.HTTPClient.Transport.(originPinTripper)
	if !ok {
		t.Fatalf("the origin pin is not outermost: %T", c.HTTPClient.Transport)
	}
	if pin.next != http.DefaultTransport {
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
		"https://COMPANY.ScreenConnect.com":                  "https://company.screenconnect.com:443",
		"https://company.screenconnect.com:8443":             "https://company.screenconnect.com:8443",
		"http://127.0.0.1/x":                                 "http://127.0.0.1:80",
		"https://webservices5.autotаsk.net/atservicesrest/x": "",
		"https://[::1]:9443/atservicesrest/V1.0":             "https://[::1]:9443",
	} {
		u, _ := url.Parse(raw)
		if got := normalizedOrigin(u); got != want {
			t.Errorf("%s: got %q, want %q", raw, got, want)
		}
	}
}

// The Basic header reaches the wire with the right pair from every source:
// the credential store (auth set-credentials), the environment (CONNECTWISE_CONTROL_USERNAME and
// CONNECTWISE_CONTROL_PASSWORD; it wins over the store) and a 0600 config
// file's control_username and control_password.
func TestTokenFromEverySourceReachesTheWire(t *testing.T) {
	const storedUser, storedPass = "storedUnitUser-9f7d5b3a1c@example.com", "storedUnitPass-5e7a9c1b3d"
	const envUser, envPass = "envUnitUser-6b8d0f2a4c@example.com", "envUnitPass-1d3f5b7a9c"
	const fileUser, filePass = "fileUnitUser-1a3c5e7b9d@example.com", "fileUnitPass-7b9d1f3a5c"
	basic := func(u, p string) string { return "Basic " + base64.StdEncoding.EncodeToString([]byte(u+":"+p)) }
	var got atomic.Value
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.Store(r.Header.Get("Authorization"))
		_, _ = w.Write([]byte(`[]`))
	}))
	t.Cleanup(api.Close)
	testenv.Isolate(t)
	t.Setenv("CONNECTWISE_CONTROL_BASE_URL", api.URL)
	for _, n := range []string{"CONNECTWISE_CONTROL_USERNAME", "CONNECTWISE_CONTROL_PASSWORD"} {
		t.Setenv(n, "")
	}
	cmd := RootCmd()
	cmd.SetArgs([]string{"auth", "set-credentials", storedUser, storedPass})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("auth set-credentials: %v", err)
	}
	flags := rootFlags{timeout: 10 * time.Second}
	wire := func(want, what string) {
		t.Helper()
		c, err := flags.newClient()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := c.Get(context.Background(), "/Services/SessionGroupService.ashx/session-groups", nil); err != nil {
			t.Fatal(err)
		}
		if got.Load() != want {
			t.Fatalf("Authorization on the wire = %v, want %s", got.Load(), what)
		}
	}
	wire(basic(storedUser, storedPass), "the stored pair")
	t.Setenv("CONNECTWISE_CONTROL_USERNAME", envUser)
	t.Setenv("CONNECTWISE_CONTROL_PASSWORD", envPass)
	wire(basic(envUser, envPass), "the environment pair (it wins over the store)")
	cfgPath := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(cfgPath, []byte(fmt.Sprintf("control_username = %q\ncontrol_password = %q\n", fileUser, filePass)), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"CONNECTWISE_CONTROL_USERNAME", "CONNECTWISE_CONTROL_PASSWORD"} {
		t.Setenv(n, "")
	}
	t.Setenv("CONNECTWISE_CONTROL_CONFIG", cfgPath)
	wire(basic(fileUser, filePass), "the config file pair")
}
