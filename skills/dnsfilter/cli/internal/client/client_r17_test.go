// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/dnsfilter/handfixes.json
// under the "config-headers-never-replace-auth",
// "redirect-strips-every-custom-credential",
// "response-text-in-errors-is-masked" and "response-cache-never-holds-secrets"
// entries.

package client

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"dnsfilter-pp-cli/internal/cliutil"
	"dnsfilter-pp-cli/internal/cliutil/testenv"
	"dnsfilter-pp-cli/internal/config"
)

// captureR17Stderr runs fn with os.Stderr redirected and returns what it
// wrote. cliutil.Stderr resolves os.Stderr on every write.
func captureR17Stderr(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w
	done := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		done <- buf.String()
	}()
	var out string
	func() {
		defer func() {
			os.Stderr = old
			_ = w.Close()
			out = <-done
			_ = r.Close()
		}()
		fn()
	}()
	return out
}

// An Authorization entry under [headers] never replaces the key set-token
// saved, never authenticates a logged-out config, is warned about once, and
// a credential carried in any config-file header is masked in errors.
func TestConfigHeadersNeverReplaceTheAuthHeader(t *testing.T) {
	var mu sync.Mutex
	var seen []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, r.Header.Get("Authorization"))
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"bad request","echo":"` + r.Header.Get("X-Proxy-Token") + `"}`))
	}))
	t.Cleanup(server.Close)
	configAuthorizationWarned = sync.Once{}
	headers := map[string]string{"authorization": "stale-key-r17", "X-Proxy-Token": "proxy-secret-r17"}

	var errText string
	stderr := captureR17Stderr(t, func() {
		c := New(&config.Config{BaseURL: server.URL, DnsfilterApiKey: "saved-key-r17", Headers: headers, Path: "/tenant/config.toml"}, 5*time.Second, 0)
		c.NoCache = true
		for i := 0; i < 2; i++ {
			_, err := c.Get(context.Background(), "/v1/organizations", nil)
			if err == nil {
				t.Fatal("expected the 400 to fail")
			}
			errText = err.Error()
		}
		loggedOut := New(&config.Config{BaseURL: server.URL, Headers: headers}, 5*time.Second, 0)
		loggedOut.NoCache = true
		_, _ = loggedOut.Get(context.Background(), "/v1/organizations", nil)
	})
	mu.Lock()
	defer mu.Unlock()
	if len(seen) != 3 || seen[0] != "saved-key-r17" || seen[1] != "saved-key-r17" {
		t.Fatalf("Authorization sent = %q, want the saved key, never the [headers] entry", seen)
	}
	if seen[2] != "" {
		t.Fatalf("a logged-out config still sent Authorization %q from [headers]", seen[2])
	}
	if strings.Contains(errText, "proxy-secret-r17") || strings.Contains(errText, "stale-key-r17") {
		t.Fatalf("a credential from [headers] reached the error text: %s", errText)
	}
	if n := strings.Count(stderr, "ignoring the Authorization entry under [headers]"); n != 1 {
		t.Fatalf("[headers] Authorization warning printed %d times, want once:\n%s", n, stderr)
	}
}

// r17Transport answers in memory: the configured API host redirects
// /v1/away to another host and /v1/near to itself; every other request is
// a 200 {} that records the headers it arrived with.
type r17Transport struct {
	mu   sync.Mutex
	seen map[string]http.Header
}

func (rt *r17Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	rt.mu.Lock()
	rt.seen[req.URL.Host+req.URL.Path] = req.Header.Clone()
	rt.mu.Unlock()
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{}`)), Request: req}
	switch {
	case req.URL.Host == "api.dnsfilter.test" && req.URL.Path == "/v1/away":
		resp.StatusCode = http.StatusFound
		resp.Header.Set("Location", "https://elsewhere.test/landing")
	case req.URL.Host == "api.dnsfilter.test" && req.URL.Path == "/v1/near":
		resp.StatusCode = http.StatusFound
		resp.Header.Set("Location", "/v1/landed")
	}
	return resp, nil
}

// A redirect off the configured API host carries none of the config-file
// headers and no credential-named header; a same-host hop keeps them. The
// response URL recorder reports where the answer came from.
func TestRedirectOffOriginStripsEveryCustomCredential(t *testing.T) {
	rt := &r17Transport{seen: map[string]http.Header{}}
	cfg := &config.Config{
		BaseURL:         "https://api.dnsfilter.test",
		DnsfilterApiKey: "saved-key-r17",
		Headers:         map[string]string{"X-Api-Key": "proxy-secret-r17", "X-Org-Id": "42"},
	}
	c := New(cfg, 5*time.Second, 0)
	c.NoCache = true
	c.HTTPClient.Transport = rt
	var final url.URL
	ctx := WithResponseURLRecorder(context.Background(), &final)
	if _, err := c.GetWithHeaders(ctx, "/v1/away", nil, map[string]string{"X-Session-Token": "session-r17", "Accept": "application/json"}); err != nil {
		t.Fatal(err)
	}
	if final.Host != "elsewhere.test" || final.Path != "/landing" {
		t.Fatalf("recorded response URL = %q, want https://elsewhere.test/landing", final.String())
	}
	if _, err := c.Get(context.Background(), "/v1/near", nil); err != nil {
		t.Fatal(err)
	}
	rt.mu.Lock()
	defer rt.mu.Unlock()
	away := rt.seen["elsewhere.test/landing"]
	if away == nil {
		t.Fatal("the cross-host hop was not followed")
	}
	for _, name := range []string{"Authorization", "X-Api-Key", "X-Org-Id", "X-Session-Token"} {
		if v := away.Get(name); v != "" {
			t.Errorf("%s reached another host on a redirect: %q", name, v)
		}
	}
	if away.Get("Accept") == "" {
		t.Error("a non-credential request header was stripped too")
	}
	near := rt.seen["api.dnsfilter.test/v1/landed"]
	if near == nil || near.Get("X-Api-Key") != "proxy-secret-r17" || near.Get("Authorization") != "saved-key-r17" {
		t.Fatalf("a same-host redirect dropped the configured headers: %v", near)
	}
}

// Every response-derived string an error carries (an HTML title, a
// Content-Encoding value) is masked like an HTTP error body.
func TestResponseTextInErrorsIsMasked(t *testing.T) {
	const key = "Secret-Key-R17" // mixed case: the Content-Encoding error must not fold it past the masker
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/html":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<!doctype html><html><head><title>Your key ` + key + `</title></head><body>x</body></html>`))
		case "/v1/encoding":
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Content-Encoding", "x-"+key)
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	t.Cleanup(server.Close)
	c := New(&config.Config{BaseURL: server.URL, DnsfilterApiKey: key}, 5*time.Second, 0)
	c.NoCache = true
	for _, p := range []string{"/v1/html", "/v1/encoding"} {
		_, err := c.Get(context.Background(), p, nil)
		if err == nil {
			t.Fatalf("GET %s: want an error", p)
		}
		if strings.Contains(strings.ToLower(err.Error()), strings.ToLower(key)) {
			t.Fatalf("GET %s: the API key (in any case) reached the error text: %v", p, err)
		}
	}
}

// A non-JSON 2xx body is never cached, and a cached one (an older build
// wrote it) is never served: under the read-only switch the request reaches
// the network path, which refuses the body.
func TestCacheHitNeverSkipsReadOnlyRefusal(t *testing.T) {
	testenv.Isolate(t, cliutil.CacheDir)
	t.Setenv(cliutil.MCPReadOnlyEnvVar, "")
	const body = `{<network><secret_key>NET-SECRET-R17</secret_key></network>`
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	c := New(&config.Config{BaseURL: server.URL, DnsfilterApiKey: "k"}, 5*time.Second, 0)
	if _, err := c.Get(context.Background(), "/v1/networks/7", nil); err != nil {
		t.Fatalf("normal mode GET: %v", err)
	}
	t.Setenv(cliutil.MCPReadOnlyEnvVar, "1")
	if data, err := c.Get(context.Background(), "/v1/networks/7", nil); err == nil || strings.Contains(string(data), "NET-SECRET-R17") {
		t.Fatalf("read-only GET after a normal-mode run returned %q (%v), want the non-JSON refusal", data, err)
	}

	// A file an older build cached is not served either.
	cacheFile := filepath.Join(c.cacheResourceDir("/v1/networks/8"), c.cacheKeyFor(http.MethodGet, "/v1/networks/8", nil, nil, nil)+".json")
	if err := os.MkdirAll(filepath.Dir(cacheFile), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cacheFile, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	before := hits.Load()
	if data, err := c.Get(context.Background(), "/v1/networks/8", nil); err == nil || strings.Contains(string(data), "NET-SECRET-R17") {
		t.Fatalf("read-only GET served a cached non-JSON body: %q (%v)", data, err)
	}
	if hits.Load() == before {
		t.Fatal("the cached non-JSON body answered the request instead of the network path")
	}
	if _, err := os.Stat(cacheFile); !os.IsNotExist(err) {
		t.Fatalf("the cached non-JSON body was left in place (%v)", err)
	}
}
