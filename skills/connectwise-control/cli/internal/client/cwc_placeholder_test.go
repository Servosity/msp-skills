// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated): handfixes.json no-credential-to-placeholder-host.

package client

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"connectwise-control-pp-cli/internal/cliutil"
	"connectwise-control-pp-cli/internal/cliutil/testenv"
	"connectwise-control-pp-cli/internal/config"
)

// Fixture credentials: random, and sharing no 8-character piece with the
// connector's program text.
const (
	placeholderTestUser = "M8ulUDUpJUtPpcJnwl"
	placeholderTestPass = "7QxNq7PWgKblc6ln3v"
)

// wireRecorder stands in for the network: it records every request that
// reached the transport and answers none of them.
type wireRecorder struct {
	mu   sync.Mutex
	seen []string
}

func (r *wireRecorder) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Body != nil {
		_ = req.Body.Close()
	}
	r.mu.Lock()
	r.seen = append(r.seen, req.Method+" "+req.URL.Host+" auth="+req.Header.Get("Authorization"))
	r.mu.Unlock()
	return nil, errors.New("wire recorder: the request reached the transport")
}

func (r *wireRecorder) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.seen)
}

// placeholderSpellings is the shipped placeholder base URL in every spelling
// that still names its host: case, a trailing dot, a port, userinfo, a path,
// plain http, and an unset (empty) base URL.
var placeholderSpellings = []string{
	"https://your-instance.screenconnect.com",
	"https://your-instance.screenconnect.com/",
	"HTTPS://YOUR-INSTANCE.ScreenConnect.COM",
	"https://your-instance.screenconnect.com.",
	"https://your-instance.screenconnect.com:443",
	"https://your-instance.screenconnect.com:8040/",
	"https://Your-Instance.screenconnect.com.:443/App_Extensions",
	"https://ops@your-instance.screenconnect.com",
	"https://ops:pw@YOUR-instance.screenconnect.com./",
	"http://your-instance.screenconnect.com",
	"",
	"   ",
}

// newPlaceholderClient builds a client for base with the fixture credential and
// swaps its transport for the recorder. The response cache directory resolves
// inside a sandboxed home.
func newPlaceholderClient(t *testing.T, base string) (*Client, *wireRecorder) {
	t.Helper()
	cfg := &config.Config{
		BaseURL:                    base,
		ConnectwiseControlUsername: placeholderTestUser,
		ConnectwiseControlPassword: placeholderTestPass,
	}
	c := New(cfg, 5*time.Second, 0)
	if c.HTTPClient == nil {
		t.Fatalf("%q: no HTTP client: %v", base, c.TLSConfigError())
	}
	rec := &wireRecorder{}
	c.HTTPClient.Transport = rec
	c.NoCache = true
	return c, rec
}

// requirePlaceholderRefusal: the error names the variable and the config key
// that fix it.
func requirePlaceholderRefusal(t *testing.T, what string, err error) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s: no error; a request with the shipped placeholder base URL must be refused", what)
	}
	msg := err.Error()
	for _, want := range []string{"CONNECTWISE_CONTROL_BASE_URL", "base_url"} {
		if !strings.Contains(msg, want) {
			t.Errorf("%s: the refusal does not name %s: %v", what, want, err)
		}
	}
	if strings.Contains(msg, placeholderTestUser) || strings.Contains(msg, placeholderTestPass) {
		t.Errorf("%s: the refusal carries the credential: %v", what, err)
	}
}

// Every verb a command can use (a GET endpoint, the novels' read-only POST, a
// mutating POST, a cache-bypassing GET) is refused before the transport when
// the base URL is the shipped placeholder, whatever its spelling.
func TestPlaceholderBaseURLRefusedBeforeTheTransport(t *testing.T) {
	testenv.Isolate(t, cliutil.CacheDir)
	for _, base := range placeholderSpellings {
		c, rec := newPlaceholderClient(t, base)
		// A send the refusal misses is retried with backoff; the deadline keeps
		// that failure fast.
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		_, err := c.Get(ctx, "/Services/SessionGroupService.ashx/session-groups", nil)
		requirePlaceholderRefusal(t, base+" GET", err)
		_, _, err = c.PostQueryWithParams(ctx, "/Services/PageService.ashx/GetLiveData", map[string]string{}, []any{map[string]any{"HostSessionInfo": map[string]any{"sessionType": 2}}})
		requirePlaceholderRefusal(t, base+" read POST", err)
		_, _, err = c.Post(ctx, "/Services/AuditService.ashx/GetAuditInfo", map[string]any{})
		requirePlaceholderRefusal(t, base+" POST", err)
		_, err = c.GetNoCache(ctx, "/Services/SessionGroupService.ashx/session-groups", nil)
		requirePlaceholderRefusal(t, base+" GET (no cache)", err)
		cancel()
		if n := rec.count(); n != 0 {
			t.Errorf("%q: %d request(s) reached the transport: %v", base, n, rec.seen)
		}
	}
}

// The refusal is about the placeholder host, nothing wider: a real instance
// on the same cloud domain, an on-premises host, and a host that merely
// contains the placeholder's name all still reach the transport.
func TestRealInstanceStillReachesTheTransport(t *testing.T) {
	testenv.Isolate(t, cliutil.CacheDir)
	for _, base := range []string{
		"https://cwc-fixture-tenant.screenconnect.com",
		"https://control.msp-fixture.test:8040",
		"https://your-instance.screenconnect.com.fixture.test",
		"https://x.your-instance.screenconnect.com",
	} {
		c, rec := newPlaceholderClient(t, base)
		// A mutating POST is never retried, so the recorder's error returns at once.
		_, _, err := c.Post(context.Background(), "/Services/AuditService.ashx/GetAuditInfo", map[string]any{})
		if err == nil || !strings.Contains(err.Error(), "wire recorder") {
			t.Errorf("%s: want the request at the transport, got %v", base, err)
		}
		if rec.count() == 0 {
			t.Errorf("%s: a configured instance never reached the transport", base)
		}
	}
}
