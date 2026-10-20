// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored. Exercises the ESET Connect password-grant mint, the
// tenant-identity binding of the cached token, and the 202/401 transport.
package cli

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"eset-protect-pp-cli/internal/client"
	"eset-protect-pp-cli/internal/cliutil/testenv"
	"eset-protect-pp-cli/internal/config"
)

func newEsetTokenServer(t *testing.T, mints *int32) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		switch {
		case r.Form.Get("grant_type") == "password" && r.Form.Get("username") == "api@msp.example" && r.Form.Get("password") == "s3cret":
		case r.Form.Get("grant_type") == "refresh_token" && r.Form.Get("refresh_token") == "refresh-1":
		default:
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"error":"unauthorized_client","error_description":"invalid_username_or_password"}`)
			return
		}
		n := atomic.AddInt32(mints, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"tok-`+strconv.Itoa(int(n))+`","refresh_token":"refresh-1","token_type":"Bearer","expires_in":3600}`)
	}))
}

// esetTestConfig isolates HOME/XDG so SaveTokens writes stay per test.
func esetTestConfig(t *testing.T) *config.Config {
	t.Helper()
	testenv.Isolate(t)
	cfg, err := config.Load(t.TempDir() + "/config.toml")
	if err != nil {
		t.Fatal(err)
	}
	cfg.EsetProtectUsername = "api@msp.example"
	cfg.EsetProtectPassword = "s3cret"
	cfg.EsetProtectToken = ""
	cfg.AccessToken, cfg.RefreshToken, cfg.ClientID = "", "", ""
	cfg.TemplateVars = map[string]string{"region": "us"}
	return cfg
}

func withTokenServer(t *testing.T) *int32 {
	t.Helper()
	var mints int32
	srv := newEsetTokenServer(t, &mints)
	t.Cleanup(srv.Close)
	esetTokenURLOverride = srv.URL
	t.Cleanup(func() { esetTokenURLOverride = "" })
	return &mints
}

func TestEsetEnsureTokenMintsAndBindsIdentity(t *testing.T) {
	mints := withTokenServer(t)
	cfg := esetTestConfig(t)
	if err := esetEnsureToken(context.Background(), cfg, false, true); err != nil {
		t.Fatal(err)
	}
	if got := cfg.AuthHeader(); got != "Bearer tok-1" {
		t.Fatalf("AuthHeader = %q", got)
	}
	// Cached and still fresh: no second mint.
	if err := esetEnsureToken(context.Background(), cfg, false, true); err != nil || *mints != 1 {
		t.Fatalf("expected cached reuse, mints=%d err=%v", *mints, err)
	}
	// A different region (tenant) must not reuse the token.
	cfg.TemplateVars["region"] = "de"
	cfg.RefreshToken = "refresh-other"
	if err := esetEnsureToken(context.Background(), cfg, false, true); err != nil {
		t.Fatal(err)
	}
	if *mints != 2 || cfg.ClientID != esetIdentity("de", "api@msp.example") {
		t.Fatalf("expected a fresh mint bound to de, mints=%d id=%q", *mints, cfg.ClientID)
	}
	// An explicit ESET_PROTECT_TOKEN wins over the cached minted token.
	cfg.EsetProtectToken = "explicit"
	_ = esetEnsureToken(context.Background(), cfg, false, true)
	if got := cfg.AuthHeader(); got != "Bearer explicit" {
		t.Fatalf("explicit token did not win: %q", got)
	}
}

func TestEsetForeignTokenDroppedWithoutPassword(t *testing.T) {
	withTokenServer(t)
	cfg := esetTestConfig(t)
	if err := esetEnsureToken(context.Background(), cfg, false, true); err != nil {
		t.Fatal(err)
	}
	// User B, no password: user A's minted token must not be sent.
	cfg.EsetProtectUsername, cfg.EsetProtectPassword = "other@msp.example", ""
	if err := esetEnsureToken(context.Background(), cfg, false, true); err != nil {
		t.Fatal(err)
	}
	if got := cfg.AuthHeader(); got != "" {
		t.Fatalf("token minted for another user leaked: %q", got)
	}
	// No credentials at all but a different region: also dropped.
	cfg2 := esetTestConfig(t)
	if err := esetEnsureToken(context.Background(), cfg2, false, true); err != nil {
		t.Fatal(err)
	}
	cfg2.EsetProtectUsername, cfg2.EsetProtectPassword = "", ""
	cfg2.TemplateVars["region"] = "jpn"
	_ = esetEnsureToken(context.Background(), cfg2, false, true)
	if got := cfg2.AuthHeader(); got != "" {
		t.Fatalf("token minted for region us leaked into jpn: %q", got)
	}
	// A hand-stored token (auth set-token) carries no stamp and is kept.
	cfg3 := esetTestConfig(t)
	cfg3.EsetProtectUsername, cfg3.EsetProtectPassword = "", ""
	cfg3.AccessToken = "manual"
	_ = esetEnsureToken(context.Background(), cfg3, false, true)
	if got := cfg3.AuthHeader(); got != "Bearer manual" {
		t.Fatalf("set-token value was dropped: %q", got)
	}
}

func TestEsetEnsureTokenRejectsBadPasswordAndRegion(t *testing.T) {
	withTokenServer(t)
	cfg := esetTestConfig(t)
	cfg.EsetProtectPassword = "wrong"
	err := esetEnsureToken(context.Background(), cfg, false, true)
	if err == nil || !strings.Contains(err.Error(), "invalid_username_or_password") {
		t.Fatalf("want rejection naming the reason, got %v", err)
	}
	if strings.Contains(err.Error(), "wrong") {
		t.Fatal("error leaked the password")
	}
	cfg.TemplateVars["region"] = "evil.example"
	if err := esetEnsureToken(context.Background(), cfg, false, true); err == nil {
		t.Fatal("expected invalid region to be refused before any request")
	}
}

func esetFastPolling(t *testing.T) {
	t.Helper()
	prev := client.PendingPollStart
	client.PendingPollStart = time.Millisecond
	t.Cleanup(func() { client.PendingPollStart = prev })
}

func TestEsetTransportPollsPendingAndReplays401(t *testing.T) {
	withTokenServer(t)
	esetFastPolling(t)
	var calls int32
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		body, _ := io.ReadAll(r.Body)
		switch {
		case n == 1: // stale token
			w.WriteHeader(http.StatusUnauthorized)
		case r.Header.Get("response-id") == "" && n == 2:
			if r.Header.Get("Authorization") != "Bearer tok-2" || string(body) != `{"x":1}` {
				t.Errorf("replay lost auth or body: %q %q", r.Header.Get("Authorization"), body)
			}
			w.Header().Set("response-id", "pending-1")
			w.WriteHeader(http.StatusAccepted)
		case r.Header.Get("response-id") == "pending-1" && string(body) == `{"x":1}` && r.Header.Get("Authorization") == "Bearer tok-2":
			_, _ = io.WriteString(w, `{"ok":true}`)
		default:
			t.Errorf("unexpected request %d", n)
			w.WriteHeader(http.StatusTeapot)
		}
	}))
	defer api.Close()

	cfg := esetTestConfig(t)
	if err := esetEnsureToken(context.Background(), cfg, false, true); err != nil {
		t.Fatal(err)
	}
	tr := &client.EsetTransport{Base: http.DefaultTransport}
	tr.Reauth = esetReauth(cfg, tr)
	req, _ := http.NewRequest(http.MethodPost, api.URL+"/v1/devices:batchGet", strings.NewReader(`{"x":1}`))
	req.Header.Set("Authorization", cfg.AuthHeader())
	resp, err := tr.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK || string(got) != `{"ok":true}` || calls != 3 {
		t.Fatalf("status=%d body=%s calls=%d", resp.StatusCode, got, calls)
	}
	// The shared config was not mutated by the concurrent-safe re-mint.
	if cfg.AuthHeader() != "Bearer tok-1" || tr.Auth() != "Bearer tok-2" {
		t.Fatalf("re-mint should publish via the transport: cfg=%q transport=%q", cfg.AuthHeader(), tr.Auth())
	}
}

func TestEsetTransport401DuringPollingReauths(t *testing.T) {
	withTokenServer(t)
	esetFastPolling(t)
	var calls int32
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		switch {
		case n == 1:
			w.Header().Set("response-id", "p")
			w.WriteHeader(http.StatusAccepted)
		case n == 2: // token expired while pending
			w.WriteHeader(http.StatusUnauthorized)
		case r.Header.Get("response-id") == "p" && r.Header.Get("Authorization") == "Bearer tok-2":
			_, _ = io.WriteString(w, `{"ok":true}`)
		default:
			w.WriteHeader(http.StatusTeapot)
		}
	}))
	defer api.Close()
	cfg := esetTestConfig(t)
	_ = esetEnsureToken(context.Background(), cfg, false, true)
	tr := &client.EsetTransport{Base: http.DefaultTransport}
	tr.Reauth = esetReauth(cfg, tr)
	req, _ := http.NewRequest(http.MethodGet, api.URL+"/v1/devices", nil)
	req.Header.Set("Authorization", cfg.AuthHeader())
	resp, err := tr.RoundTrip(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("err=%v status=%v calls=%d", err, resp, calls)
	}
}

func TestEsetTransportPendingTimeoutIsAnError(t *testing.T) {
	esetFastPolling(t)
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("response-id", "forever")
		w.WriteHeader(http.StatusAccepted)
	}))
	defer api.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, api.URL+"/v1/devices", nil)
	resp, err := (&client.EsetTransport{}).RoundTrip(req)
	if err == nil || resp != nil {
		t.Fatalf("a request that never leaves 202 must fail, got resp=%v err=%v", resp, err)
	}
	var pending *client.ErrPendingTimeout
	if !errors.As(err, &pending) && !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("unexpected error type: %v", err)
	}
}

func TestEsetTransport202WithoutResponseIDIsAnError(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		_, _ = io.WriteString(w, `{}`)
	}))
	defer api.Close()
	req, _ := http.NewRequest(http.MethodGet, api.URL+"/v1/devices", nil)
	resp, err := (&client.EsetTransport{}).RoundTrip(req)
	if err == nil || resp != nil || !strings.Contains(err.Error(), "response-id") {
		t.Fatalf("an uncollectable 202 must be an error, got resp=%v err=%v", resp, err)
	}
}

func TestEsetStoreIdentityIsStableAcrossTokens(t *testing.T) {
	cfg := esetTestConfig(t)
	cfg.AccessToken = "tok-1"
	first := esetStoreIdentity(cfg)
	cfg.AccessToken = "tok-2" // an hourly refresh must not move the mirror
	if first == "" || esetStoreIdentity(cfg) != first || cfg.StoreScopeCredential() != first {
		t.Fatalf("store identity changed with the token: %q -> %q", first, esetStoreIdentity(cfg))
	}
	cfg.TemplateVars["region"] = "de"
	if esetStoreIdentity(cfg) == first {
		t.Fatal("a different region must use a different mirror")
	}
	cfg.EsetProtectUsername = ""
	cfg.EsetProtectToken = "explicit"
	if got := esetStoreIdentity(cfg); got != "" {
		t.Fatalf("a bare token falls back to the generated scope, got %q", got)
	}
}

func TestEsetClientHookRejectsInjectedRegionWithExplicitToken(t *testing.T) {
	cfg := esetTestConfig(t)
	cfg.EsetProtectUsername, cfg.EsetProtectPassword = "", ""
	cfg.EsetProtectToken = "explicit"
	for _, region := range []string{"eu@attacker.example/x", "evil.example", "us.attacker"} {
		cfg.TemplateVars["region"] = region
		if err := esetClientHook(client.New(cfg, 5*time.Second, 0)); err == nil {
			t.Errorf("region %q with an explicit token must be refused", region)
		}
		// A dry run sends nothing, so it may preview any region.
		dry := client.New(cfg, 5*time.Second, 0)
		dry.DryRun = true
		if err := esetClientHook(dry); err != nil {
			t.Errorf("dry run with region %q must only preview, got %v", region, err)
		}
	}
	cfg.TemplateVars["region"] = "us"
	if err := esetClientHook(client.New(cfg, 5*time.Second, 0)); err != nil {
		t.Fatalf("a valid region with an explicit token must pass: %v", err)
	}
}

type esetRecordingRT struct{ hosts []string }

func (r *esetRecordingRT) RoundTrip(req *http.Request) (*http.Response, error) {
	r.hosts = append(r.hosts, req.URL.Host)
	return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Header: http.Header{}, Request: req}, nil
}

// A flag that rewrites the URL after the region check (sync --path-context
// region=...) must not be able to carry the bearer token off ESET.
func TestEsetTransportPinsCredentialsToEsetHosts(t *testing.T) {
	cfg := esetTestConfig(t)
	cfg.EsetProtectUsername, cfg.EsetProtectPassword = "", ""
	cfg.EsetProtectToken = "explicit"
	c := client.New(cfg, 5*time.Second, 0)
	if err := esetClientHook(c); err != nil {
		t.Fatal(err)
	}
	tr, ok := c.HTTPClient.Transport.(*client.EsetTransport)
	if !ok {
		t.Fatalf("hook did not install EsetTransport, got %T", c.HTTPClient.Transport)
	}
	// The generated streaming path copies the client; the pin must survive it.
	if st, ok := client.StreamingHTTPClient(c.HTTPClient, time.Second).Transport.(*client.EsetTransport); !ok || st.AllowHost == nil {
		t.Fatal("streaming client dropped the credential host pin")
	}
	rec := &esetRecordingRT{}
	tr.Base = rec
	for _, u := range []string{"https://attacker.example/v1/devices", "https://eu.device-management.eset.systems.attacker.example/x", "https://eu.device-management.eset.systems:8443/x"} {
		req, _ := http.NewRequest(http.MethodGet, u, nil)
		req.Header.Set("Authorization", "Bearer explicit")
		if _, err := tr.RoundTrip(req); err == nil {
			t.Errorf("credentialed request to %s must be refused", u)
		}
	}
	if len(rec.hosts) != 0 {
		t.Fatalf("refused requests reached the network: %v", rec.hosts)
	}
	req, _ := http.NewRequest(http.MethodGet, "https://eu.device-management.eset.systems/v1/devices", nil)
	req.Header.Set("Authorization", "Bearer explicit")
	if _, err := tr.RoundTrip(req); err != nil {
		t.Fatalf("ESET host refused: %v", err)
	}
	if len(rec.hosts) != 1 {
		t.Fatalf("ESET request not sent, hosts=%v", rec.hosts)
	}
}

// A redirect from the token endpoint must not replay the password elsewhere.
func TestEsetTokenMintDoesNotFollowRedirects(t *testing.T) {
	var stolen int32
	thief := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&stolen, 1)
	}))
	defer thief.Close()
	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, thief.URL+"/oauth/token", http.StatusTemporaryRedirect)
	}))
	defer redirector.Close()
	esetTokenURLOverride = redirector.URL
	t.Cleanup(func() { esetTokenURLOverride = "" })
	form := url.Values{"grant_type": {"password"}, "username": {"u@example.com"}, "password": {"secret"}}
	if _, err := esetRequestToken(context.Background(), "eu", form); err == nil {
		t.Fatal("a redirected token request must fail")
	}
	if atomic.LoadInt32(&stolen) != 0 {
		t.Fatal("the password was replayed to the redirect target")
	}
}

// ESET_PROTECT_BASE_URL is operator configuration; its host stays allowed.
func TestEsetHostAllowerHonorsConfiguredBaseURL(t *testing.T) {
	cfg := esetTestConfig(t)
	allow := esetHostAllower(cfg)
	if allow("127.0.0.1:8080") || !allow("eu.device-management.eset.systems") {
		t.Fatal("default config must pin to ESET hosts only")
	}
	cfg.BaseURL = "http://127.0.0.1:8080"
	if !esetHostAllower(cfg)("127.0.0.1:8080") || esetHostAllower(cfg)("attacker.example") {
		t.Fatal("a configured base URL allows exactly its own host")
	}
}
