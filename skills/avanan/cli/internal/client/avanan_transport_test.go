// Copyright 2026 geekbrownbear and contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"avanan-pp-cli/internal/avanansig"
	"avanan-pp-cli/internal/config"
)

func newTestTransport(t *testing.T, base http.RoundTripper, baseURL string) (*AvananTransport, *config.Config) {
	t.Helper()
	cfg := &config.Config{
		BaseURL:      baseURL,
		AvananAppId:  "US:testapp",
		ClientSecret: "test_secret",
	}
	return &AvananTransport{base: base, cfg: cfg}, cfg
}

// TestLegacyRequestIsSignedOverFinalURL is the regression guard for the
// failure mode this transport exists to prevent: signing a different string
// than the one sent. The signature must validate against the path AND the
// encoded query as they appear on the wire.
func TestLegacyRequestIsSignedOverFinalURL(t *testing.T) {
	var got *http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1.0/auth":
			_, _ = w.Write([]byte("jwt-token-value"))
		default:
			got = r.Clone(r.Context())
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"responseData":[]}`))
		}
	}))
	defer srv.Close()

	tr, cfg := newTestTransport(t, http.DefaultTransport, srv.URL)
	httpClient := &http.Client{Transport: tr}

	resp, err := httpClient.Get(srv.URL + "/v1.0/event/query?scope=farm%3Atenant&limit=10")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if got == nil {
		t.Fatal("data request never reached the server")
	}

	reqID := got.Header.Get("x-av-req-id")
	date := got.Header.Get("x-av-date")
	sig := got.Header.Get("x-av-sig")

	if reqID == "" || date == "" || sig == "" {
		t.Fatalf("missing signing headers: req-id=%q date=%q sig=%q", reqID, date, sig)
	}
	if got.Header.Get("x-av-app-id") != cfg.AvananAppId {
		t.Errorf("x-av-app-id = %q, want %q", got.Header.Get("x-av-app-id"), cfg.AvananAppId)
	}
	if got.Header.Get("x-av-token") != "jwt-token-value" {
		t.Errorf("x-av-token = %q, want the minted token", got.Header.Get("x-av-token"))
	}

	want := avanansig.Sign(reqID, cfg.AvananAppId, date, avanansig.RequestString(got.URL), cfg.ClientSecret)
	if sig != want {
		t.Errorf("x-av-sig = %q, want %q — the signature does not cover the URL that was actually sent", sig, want)
	}
}

// TestHandshakeSignsEmptyRequestString pins the one documented asymmetry: the
// token handshake signs no request string, every other call signs one.
func TestHandshakeSignsEmptyRequestString(t *testing.T) {
	var authReq *http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1.0/auth" {
			authReq = r.Clone(r.Context())
			_, _ = w.Write([]byte("tok"))
			return
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	tr, cfg := newTestTransport(t, http.DefaultTransport, srv.URL)
	httpClient := &http.Client{Transport: tr}
	resp, err := httpClient.Get(srv.URL + "/v1.0/scopes")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if authReq == nil {
		t.Fatal("handshake never happened")
	}
	reqID := authReq.Header.Get("x-av-req-id")
	date := authReq.Header.Get("x-av-date")
	want := avanansig.Sign(reqID, cfg.AvananAppId, date, "", cfg.ClientSecret)
	if got := authReq.Header.Get("x-av-sig"); got != want {
		t.Errorf("handshake x-av-sig = %q, want %q (empty request string)", got, want)
	}
	if got := authReq.Header.Get("x-av-token"); got != "" {
		t.Errorf("handshake sent x-av-token = %q, want empty", got)
	}
}

// TestTokenIsMintedOnceAndReused guards against re-handshaking on every call,
// which would triple request volume against a rate-limited API.
func TestTokenIsMintedOnceAndReused(t *testing.T) {
	var handshakes int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1.0/auth" {
			atomic.AddInt32(&handshakes, 1)
			_, _ = w.Write([]byte("tok"))
			return
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	tr, _ := newTestTransport(t, http.DefaultTransport, srv.URL)
	httpClient := &http.Client{Transport: tr}
	for i := 0; i < 3; i++ {
		resp, err := httpClient.Get(srv.URL + "/v1.0/scopes")
		if err != nil {
			t.Fatalf("request %d: %v", i, err)
		}
		resp.Body.Close()
	}

	if n := atomic.LoadInt32(&handshakes); n != 1 {
		t.Errorf("handshakes = %d, want 1 (token should be cached across calls)", n)
	}
}

// TestStaleTokenTriggersSingleRetry covers the hour-boundary case: a cached
// token expires mid-session, the server answers 401, and the transport
// re-mints and retries exactly once.
func TestStaleTokenTriggersSingleRetry(t *testing.T) {
	var handshakes, dataCalls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1.0/auth" {
			n := atomic.AddInt32(&handshakes, 1)
			_, _ = w.Write([]byte("tok-" + string(rune('0'+n))))
			return
		}
		atomic.AddInt32(&dataCalls, 1)
		if r.Header.Get("x-av-token") == "stale" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"message":"Unauthorized"}`))
			return
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	tr, _ := newTestTransport(t, http.DefaultTransport, srv.URL)
	// Seed a token that the server will reject, as if it had just expired.
	tr.token = "stale"
	tr.expiry = timeFarFuture()

	httpClient := &http.Client{Transport: tr}
	resp, err := httpClient.Get(srv.URL + "/v1.0/scopes")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200 after refresh-and-retry", resp.StatusCode)
	}
	if n := atomic.LoadInt32(&handshakes); n != 1 {
		t.Errorf("handshakes = %d, want 1", n)
	}
	if n := atomic.LoadInt32(&dataCalls); n != 2 {
		t.Errorf("data calls = %d, want 2 (original + one retry)", n)
	}
}

// TestNoCredentialsPassesThrough keeps --help, --dry-run, and unauthenticated
// probes working instead of failing locally before they reach the wire.
func TestNoCredentialsPassesThrough(t *testing.T) {
	var got *http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Clone(r.Context())
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	tr := &AvananTransport{base: http.DefaultTransport, cfg: &config.Config{BaseURL: srv.URL}}
	httpClient := &http.Client{Transport: tr}
	resp, err := httpClient.Get(srv.URL + "/v1.0/scopes")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if got == nil {
		t.Fatal("request never reached the server")
	}
	// The generated client seeds x-av-token from AuthHeader(); with no app id
	// there is nothing meaningful to send and the placeholder must be dropped.
	if v := got.Header.Get("x-av-token"); v != "" {
		t.Errorf("x-av-token = %q, want empty when no credentials are configured", v)
	}
	if v := got.Header.Get("x-av-req-id"); v == "" {
		t.Error("x-av-req-id should still be set for request correlation")
	}
}

// TestInfinityHostUsesBearerAndPathPrefix covers the second auth generation.
func TestInfinityHostUsesBearerAndPathPrefix(t *testing.T) {
	tr, cfg := newTestTransport(t, nil, "https://cloudinfra-gw.portal.checkpoint.com")
	req, err := http.NewRequest(http.MethodGet, "https://cloudinfra-gw.portal.checkpoint.com/v1.0/scopes", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.URL.Path = avanansig.ApplyInfinityPrefix(req.URL.Path)
	req.Header.Set("x-av-token", "should-be-removed")

	tr.applyAuth(req, "bearer-token", true)

	if got := req.Header.Get("Authorization"); got != "Bearer bearer-token" {
		t.Errorf("Authorization = %q, want %q", got, "Bearer bearer-token")
	}
	if got := req.Header.Get("x-av-token"); got != "" {
		t.Errorf("x-av-token = %q, want it removed on Infinity hosts", got)
	}
	if got := req.Header.Get("x-av-sig"); got != "" {
		t.Errorf("x-av-sig = %q, want no signature on Infinity hosts", got)
	}
	if !strings.HasPrefix(req.URL.Path, avanansig.InfinityPathPrefix) {
		t.Errorf("path = %q, want the %q prefix", req.URL.Path, avanansig.InfinityPathPrefix)
	}
	_ = cfg
}

func TestExtractLegacyToken(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{name: "bare text token", body: "eyJhbGciOi.abc.def", want: "eyJhbGciOi.abc.def"},
		{name: "quoted token", body: `"eyJhbGciOi.abc.def"`, want: "eyJhbGciOi.abc.def"},
		{name: "token with surrounding whitespace", body: "  tok  \n", want: "tok"},
		{name: "envelope with top-level token", body: `{"token":"tok"}`, want: "tok"},
		{name: "infinity-shaped data envelope", body: `{"data":{"token":"tok"}}`, want: "tok"},
		{name: "responseData string", body: `{"responseData":"tok"}`, want: "tok"},
		{name: "responseData object", body: `{"responseData":{"token":"tok"}}`, want: "tok"},
		{name: "empty body", body: "", want: ""},
		{name: "envelope with no token", body: `{"responseEnvelope":{"responseCode":401}}`, want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := extractLegacyToken([]byte(tt.body)); got != tt.want {
				t.Errorf("extractLegacyToken(%q) = %q, want %q", tt.body, got, tt.want)
			}
		})
	}
}

func TestIsHandshakePath(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{path: "/v1.0/auth", want: true},
		{path: "/v1.0/auth/", want: true},
		{path: "/app/hec-api/v1.0/auth", want: true},
		{path: "/v1.0/scopes", want: false},
		{path: "/v1.0/authorization", want: false},
		{path: "", want: false},
	}
	for _, tt := range tests {
		if got := isHandshakePath(tt.path); got != tt.want {
			t.Errorf("isHandshakePath(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func timeFarFuture() time.Time { return time.Now().Add(time.Hour) }
