// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored regression suite for the OAuth2 client-credentials re-mint.
//
// Before this landed, `auth login` cached an access token and an expiry that
// nothing ever read: AuthHeader() returned "Bearer "+AccessToken forever, so a
// cipp install stopped working roughly 60 minutes after login and every
// request 401'd until a human re-ran `auth login`. These tests pin the fix and,
// just as importantly, pin the ways it must NOT fire.

package client

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"cipp-pp-cli/internal/config"
)

// tokenServer is a stand-in for the Azure AD client-credentials endpoint. It
// counts grants atomically so a concurrency test can assert an exact number of
// mints, and records the last form it was posted.
type tokenServer struct {
	srv       *httptest.Server
	grants    int64
	expiresIn int
	delay     time.Duration

	mu        sync.Mutex
	lastForm  map[string]string
	statusOut int
}

func newTokenServer(t *testing.T, expiresIn int) *tokenServer {
	t.Helper()
	ts := &tokenServer{expiresIn: expiresIn, statusOut: http.StatusOK}
	ts.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		ts.mu.Lock()
		ts.lastForm = map[string]string{}
		for k := range r.Form {
			ts.lastForm[k] = r.Form.Get(k)
		}
		ts.mu.Unlock()
		if ts.delay > 0 {
			time.Sleep(ts.delay)
		}
		n := atomic.AddInt64(&ts.grants, 1)
		if ts.statusOut != http.StatusOK {
			w.WriteHeader(ts.statusOut)
			_, _ = w.Write([]byte(`{"error":"invalid_client","error_description":"secret expired"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"access_token":%q,"expires_in":%d,"token_type":"Bearer"}`, fmt.Sprintf("minted-token-%d", n), ts.expiresIn)
	}))
	t.Cleanup(ts.srv.Close)
	return ts
}

func (ts *tokenServer) grantCount() int64 { return atomic.LoadInt64(&ts.grants) }

func (ts *tokenServer) form() map[string]string {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	out := map[string]string{}
	for k, v := range ts.lastForm {
		out[k] = v
	}
	return out
}

// tenantFor turns the httptest base URL into a (authority, tenant) pair whose
// TokenEndpoint() resolves back to the stub server. The stub answers any path.
func (ts *tokenServer) tenantFor() (authority, tenant string) {
	return ts.srv.URL, "11111111-1111-1111-1111-111111111111"
}

// newTestClient builds a *Client through New() (so limiter/cacheDir are real)
// with its config rooted in a temp dir so SaveTokens writes somewhere
// disposable.
func newTestClient(t *testing.T, cfg *config.Config) *Client {
	t.Helper()
	resetMintState()
	t.Cleanup(resetMintState)
	if cfg.Path == "" {
		cfg.Path = filepath.Join(t.TempDir(), "config.toml")
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = "http://example.test"
	}
	c := New(cfg, 5*time.Second, 0)
	c.NoCache = true
	return c
}

// resetMintState clears the package-level degrade state so tests do not inherit
// each other's failures. lastMintFailure suppresses re-mints for
// mintFailureCooldown, and the two sync.Once values suppress the warnings, so a
// test that runs after a failing one would otherwise silently observe the wrong
// branch.
func resetMintState() {
	ccMu.Lock()
	defer ccMu.Unlock()
	lastMintFailure = time.Time{}
	mintFailureWarnOnce = sync.Once{}
	remintUnavailableWarnOnce = sync.Once{}
}

// captureStderr runs fn with os.Stderr redirected to a pipe and returns what
// was written. The degrade paths warn on stderr rather than returning an error,
// so the warning IS the observable behaviour and has to be asserted.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	orig := os.Stderr
	os.Stderr = w
	done := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()
	fn()
	os.Stderr = orig
	_ = w.Close()
	out := <-done
	_ = r.Close()
	return out
}

// clearVerifyEnv makes the mint gate deterministic regardless of what the
// developer's shell (or an outer verify run) exported.
func clearVerifyEnv(t *testing.T) {
	t.Helper()
	t.Setenv("PRINTING_PRESS_VERIFY", "")
	t.Setenv("PRINTING_PRESS_VERIFY_LIVE_HTTP", "")
}

func expiringConfig(t *testing.T, ts *tokenServer, expiry time.Time) *config.Config {
	t.Helper()
	authority, tenant := ts.tenantFor()
	return &config.Config{
		BaseURL:      "http://example.test",
		AccessToken:  "stale-token",
		TokenExpiry:  expiry,
		ClientID:     "00000000-0000-0000-0000-000000000000",
		ClientSecret: "s3cret",
		TenantID:     tenant,
		Authority:    authority,
		Path:         filepath.Join(t.TempDir(), "config.toml"),
	}
}

// --- the predicate -------------------------------------------------------

// TestNeedsClientCredentialsMint is the both-directions proof for the gate:
// every row that must mint, and every row that must NOT. The static-API-key
// rows are the regression that matters most: the sibling connectors' predicate
// (`AccessToken == "" -> mint`) would return true for them on every request,
// and because ccMu is package-level that would serialize the whole fan-out.
func TestNeedsClientCredentialsMint(t *testing.T) {
	future := time.Now().Add(time.Hour)
	soon := time.Now().Add(2 * time.Minute) // inside the 5-minute skew
	past := time.Now().Add(-time.Minute)

	cases := []struct {
		name string
		cfg  *config.Config
		want bool
	}{
		{"nil config", nil, false},
		{"static CIPP_API_KEY, no access token", &config.Config{CippApiKey: "static-key"}, false},
		{"static CIPP_API_KEY alongside client creds", &config.Config{CippApiKey: "static-key", ClientID: "id", ClientSecret: "sec", TokenExpiry: past}, false},
		{"explicit auth_header", &config.Config{AuthHeaderVal: "Bearer static"}, false},
		{"explicit auth_header alongside client creds", &config.Config{AuthHeaderVal: "Bearer static", ClientID: "id", ClientSecret: "sec", TokenExpiry: past}, false},
		{"no credentials at all", &config.Config{}, false},
		{"set-token install (token, no client creds, zero expiry)", &config.Config{AccessToken: "pasted"}, false},
		{"set-token install with a past expiry", &config.Config{AccessToken: "pasted", TokenExpiry: past}, false},
		{"logged out (creds wiped)", &config.Config{TenantID: "t", Authority: "a"}, false},
		{"client id without secret", &config.Config{ClientID: "id", AccessToken: "x", TokenExpiry: past}, false},
		{"fresh token", &config.Config{ClientID: "id", ClientSecret: "sec", AccessToken: "x", TokenExpiry: future}, false},
		{"zero expiry (expires_in: 0 server)", &config.Config{ClientID: "id", ClientSecret: "sec", AccessToken: "x"}, false},
		{"expired token", &config.Config{ClientID: "id", ClientSecret: "sec", AccessToken: "x", TokenExpiry: past}, true},
		{"inside the 5m skew", &config.Config{ClientID: "id", ClientSecret: "sec", AccessToken: "x", TokenExpiry: soon}, true},
		{"client creds but no token", &config.Config{ClientID: "id", ClientSecret: "sec"}, true},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if got := needsClientCredentialsMint(tc.cfg); got != tc.want {
				t.Fatalf("needsClientCredentialsMint = %v, want %v", got, tc.want)
			}
		})
	}
}

// --- the happy path ------------------------------------------------------

// TestAuthHeader_RemintsExpiredToken is the defect itself: an expired token
// must be exchanged for a fresh one and the request must carry the NEW bearer.
func TestAuthHeader_RemintsExpiredToken(t *testing.T) {
	clearVerifyEnv(t)
	ts := newTokenServer(t, 3600)
	cfg := expiringConfig(t, ts, time.Now().Add(-time.Minute))
	c := newTestClient(t, cfg)

	header, err := c.authHeader(context.Background())
	if err != nil {
		t.Fatalf("authHeader: %v", err)
	}
	if header != "Bearer minted-token-1" {
		t.Fatalf("header = %q, want the freshly minted token", header)
	}
	if got := ts.grantCount(); got != 1 {
		t.Fatalf("token endpoint saw %d grants, want 1", got)
	}
	if time.Until(cfg.TokenExpiry) < 50*time.Minute {
		t.Fatalf("expiry not advanced: %s", cfg.TokenExpiry)
	}
	// The mint must send the stored scope, not silently downgrade to a default.
	form := ts.form()
	if form["grant_type"] != "client_credentials" {
		t.Fatalf("grant_type = %q", form["grant_type"])
	}
	if want := "api://" + cfg.ClientID + "/.default"; form["scope"] != want {
		t.Fatalf("scope = %q, want %q", form["scope"], want)
	}
	// And it must be persisted, or the next process re-mints needlessly.
	data, err := os.ReadFile(cfg.Path)
	if err != nil {
		t.Fatalf("config not written: %v", err)
	}
	if !strings.Contains(string(data), "minted-token-1") {
		t.Fatalf("minted token not persisted to %s:\n%s", cfg.Path, data)
	}

	// Second call must reuse the fresh token, not mint again.
	if _, err := c.authHeader(context.Background()); err != nil {
		t.Fatalf("second authHeader: %v", err)
	}
	if got := ts.grantCount(); got != 1 {
		t.Fatalf("token endpoint saw %d grants after a second call, want 1", got)
	}
}

// TestAuthHeader_CustomScopeSurvivesRemint pins that a sovereign-cloud or
// custom-scope login is not downgraded to the api://<client-id>/.default
// default when the token is refreshed.
func TestAuthHeader_CustomScopeSurvivesRemint(t *testing.T) {
	clearVerifyEnv(t)
	ts := newTokenServer(t, 3600)
	cfg := expiringConfig(t, ts, time.Now().Add(-time.Minute))
	cfg.Scope = "api://custom-app-id/.default"
	c := newTestClient(t, cfg)

	if _, err := c.authHeader(context.Background()); err != nil {
		t.Fatalf("authHeader: %v", err)
	}
	if got := ts.form()["scope"]; got != "api://custom-app-id/.default" {
		t.Fatalf("scope = %q, want the stored custom scope", got)
	}
}

// --- concurrency ---------------------------------------------------------

// TestAuthHeader_ConcurrentMintExactlyOnce drives 32 goroutines through
// authHeader on ONE *Client, the way internal/cliutil/fanout.go's worker pool
// does. The double-checked package-level mutex must collapse them to a single
// grant; without it a --all-tenants fan-out would stampede the token endpoint
// (and Azure AD rate-limits it).
func TestAuthHeader_ConcurrentMintExactlyOnce(t *testing.T) {
	clearVerifyEnv(t)
	ts := newTokenServer(t, 3600)
	ts.delay = 40 * time.Millisecond // widen the race window
	cfg := expiringConfig(t, ts, time.Now().Add(-time.Minute))
	c := newTestClient(t, cfg)

	const workers = 32
	var wg sync.WaitGroup
	headers := make([]string, workers)
	errs := make([]error, workers)
	start := make(chan struct{})
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			headers[i], errs[i] = c.authHeader(context.Background())
		}(i)
	}
	close(start)
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("worker %d: %v", i, err)
		}
		if headers[i] != "Bearer minted-token-1" {
			t.Fatalf("worker %d got %q, want the single minted token", i, headers[i])
		}
	}
	if got := ts.grantCount(); got != 1 {
		t.Fatalf("token endpoint saw %d grants under %d concurrent workers, want exactly 1", got, workers)
	}
}

// --- the regressions the fix must NOT introduce --------------------------

// TestAuthHeader_StaticAPIKeyNeverMints is blocker 2. A CIPP_API_KEY operator
// has AccessToken == "" permanently and no client credentials; the sibling
// connectors' predicate would try to mint on every single request.
func TestAuthHeader_StaticAPIKeyNeverMints(t *testing.T) {
	clearVerifyEnv(t)
	ts := newTokenServer(t, 3600)
	authority, tenant := ts.tenantFor()
	cfg := &config.Config{
		BaseURL:    "http://example.test",
		CippApiKey: "static-api-key",
		AuthSource: "env:CIPP_API_KEY",
		// Tenant context present but no client credentials: even a config
		// carrying leftover OAuth metadata must not mint for a static key.
		TenantID:  tenant,
		Authority: authority,
		Path:      filepath.Join(t.TempDir(), "config.toml"),
	}
	c := newTestClient(t, cfg)

	for i := 0; i < 25; i++ {
		header, err := c.authHeader(context.Background())
		if err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
		if header != "Bearer static-api-key" {
			t.Fatalf("call %d header = %q, want the static key", i, header)
		}
	}
	if got := ts.grantCount(); got != 0 {
		t.Fatalf("static-key install hit the token endpoint %d times, want 0", got)
	}
}

// TestAuthHeader_StaticAPIKeyWithClientCredsNeverMints covers the operator who
// ran `auth login` once and then switched to CIPP_API_KEY. AuthHeader() gives
// the api key precedence, so minting an access token nothing will read is
// wasted network I/O behind a global lock.
func TestAuthHeader_StaticAPIKeyWithClientCredsNeverMints(t *testing.T) {
	clearVerifyEnv(t)
	ts := newTokenServer(t, 3600)
	cfg := expiringConfig(t, ts, time.Now().Add(-time.Hour))
	cfg.CippApiKey = "static-api-key"
	c := newTestClient(t, cfg)

	header, err := c.authHeader(context.Background())
	if err != nil {
		t.Fatalf("authHeader: %v", err)
	}
	if header != "Bearer static-api-key" {
		t.Fatalf("header = %q, want the static key to win", header)
	}
	if got := ts.grantCount(); got != 0 {
		t.Fatalf("token endpoint saw %d grants, want 0", got)
	}
}

// TestAuthHeader_ExpiresInZeroDoesNotLoop pins the non-conformant-server
// guard. Storing time.Now() for expires_in: 0 would make the token look
// expired the instant it arrived and re-mint on every request forever.
func TestAuthHeader_ExpiresInZeroDoesNotLoop(t *testing.T) {
	clearVerifyEnv(t)
	ts := newTokenServer(t, 0)
	cfg := expiringConfig(t, ts, time.Now().Add(-time.Minute))
	c := newTestClient(t, cfg)

	for i := 0; i < 10; i++ {
		header, err := c.authHeader(context.Background())
		if err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
		if header != "Bearer minted-token-1" {
			t.Fatalf("call %d header = %q, want the single minted token", i, header)
		}
	}
	if got := ts.grantCount(); got != 1 {
		t.Fatalf("expires_in: 0 produced %d grants over 10 calls, want 1", got)
	}
	if !cfg.TokenExpiry.IsZero() {
		t.Fatalf("expires_in: 0 must store the zero time, got %s", cfg.TokenExpiry)
	}
}

// --- the migration path for every existing install -----------------------

// TestAuthHeader_MissingTenantIDIsLoudNotSilent is blocker 1. Every config
// written before tenant_id existed reaches this branch. Returning the stale
// token would reproduce the identical unexplained 401 the operator reported.
func TestAuthHeader_MissingTenantIDIsLoudNotSilent(t *testing.T) {
	clearVerifyEnv(t)
	cfgPath := filepath.Join(t.TempDir(), "config.toml")
	cfg := &config.Config{
		BaseURL:      "http://example.test",
		AccessToken:  "expired-token",
		TokenExpiry:  time.Now().Add(-time.Minute),
		ClientID:     "00000000-0000-0000-0000-000000000000",
		ClientSecret: "s3cret",
		Path:         cfgPath,
	}
	c := newTestClient(t, cfg)

	header, err := c.authHeader(context.Background())
	if err == nil {
		t.Fatalf("expected an actionable error, got header %q", header)
	}
	if header != "" {
		t.Fatalf("stale token leaked through the error path: %q", header)
	}
	msg := err.Error()
	for _, want := range []string{"tenant_id", "auth login", "--tenant-id", "--client-id", "--base-url", cfgPath} {
		if !strings.Contains(msg, want) {
			t.Fatalf("error must name %q so the operator knows what to run; got:\n%s", want, msg)
		}
	}
	if strings.Contains(msg, "s3cret") {
		t.Fatalf("error leaked the client secret:\n%s", msg)
	}
}

// TestAuthHeader_MissingTenantIDStillValidTokenDegrades is the other
// direction: a pre-migration config whose token is inside the 5-minute skew
// but NOT yet expired still works. Hard-failing there would break a healthy
// install minutes early; the operator gets a one-time stderr warning instead.
func TestAuthHeader_MissingTenantIDStillValidTokenDegrades(t *testing.T) {
	clearVerifyEnv(t)
	cfg := &config.Config{
		BaseURL:      "http://example.test",
		AccessToken:  "still-valid-token",
		TokenExpiry:  time.Now().Add(2 * time.Minute),
		ClientID:     "00000000-0000-0000-0000-000000000000",
		ClientSecret: "s3cret",
		Path:         filepath.Join(t.TempDir(), "config.toml"),
	}
	c := newTestClient(t, cfg)

	header, err := c.authHeader(context.Background())
	if err != nil {
		t.Fatalf("a still-valid token must not hard-fail: %v", err)
	}
	if header != "Bearer still-valid-token" {
		t.Fatalf("header = %q, want the still-valid cached token", header)
	}
}

// --- transport, dry-run, verify mode -------------------------------------

// stubRoundTripper answers the token exchange without any socket. If the mint
// ever reached http.DefaultClient instead of c.HTTPClient this test's
// unroutable authority would surface as a dial error.
type stubRoundTripper struct {
	calls  int64
	hosts  []string
	mu     sync.Mutex
	status int
}

func (s *stubRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	atomic.AddInt64(&s.calls, 1)
	s.mu.Lock()
	s.hosts = append(s.hosts, req.URL.Host)
	s.mu.Unlock()
	status := s.status
	if status == 0 {
		status = http.StatusOK
	}
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(`{"access_token":"stub-token","expires_in":3600}`)),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
	}, nil
}

// TestMintUsesConfiguredHTTPClient is blocker 4. The token exchange must ride
// c.HTTPClient's transport so a stub RoundTripper (and any operator proxy or
// TLS config) applies. 127.0.0.1:1 is unroutable, so a DefaultClient mint
// would fail with a dial error rather than returning the stub token.
func TestMintUsesConfiguredHTTPClient(t *testing.T) {
	clearVerifyEnv(t)
	rt := &stubRoundTripper{}
	cfg := &config.Config{
		BaseURL:      "http://example.test",
		AccessToken:  "stale-token",
		TokenExpiry:  time.Now().Add(-time.Minute),
		ClientID:     "00000000-0000-0000-0000-000000000000",
		ClientSecret: "s3cret",
		TenantID:     "11111111-1111-1111-1111-111111111111",
		Authority:    "http://127.0.0.1:1",
		Path:         filepath.Join(t.TempDir(), "config.toml"),
	}
	c := newTestClient(t, cfg)
	c.HTTPClient = &http.Client{Transport: rt, Timeout: 5 * time.Second}

	header, err := c.authHeader(context.Background())
	if err != nil {
		t.Fatalf("mint did not use c.HTTPClient (dialed for real?): %v", err)
	}
	if header != "Bearer stub-token" {
		t.Fatalf("header = %q, want the stubbed token", header)
	}
	if got := atomic.LoadInt64(&rt.calls); got != 1 {
		t.Fatalf("stub transport saw %d calls, want 1", got)
	}
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if len(rt.hosts) != 1 || rt.hosts[0] != "127.0.0.1:1" {
		t.Fatalf("token request did not go through the configured transport: %v", rt.hosts)
	}
}

// TestMintPreservesTokenOnReadOnlyConfigDir is blocker 7. Nine sibling
// connectors return an error when the minted token cannot be persisted, which
// throws away a perfectly valid credential on a read-only home dir. cipp must
// keep working in memory for the life of the process.
func TestMintPreservesTokenOnReadOnlyConfigDir(t *testing.T) {
	clearVerifyEnv(t)
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	ts := newTokenServer(t, 3600)
	roDir := filepath.Join(t.TempDir(), "readonly")
	if err := os.MkdirAll(roDir, 0o500); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(roDir, 0o700) })
	cfg := expiringConfig(t, ts, time.Now().Add(-time.Minute))
	cfg.Path = filepath.Join(roDir, "sub", "config.toml")
	c := newTestClient(t, cfg)

	header, err := c.authHeader(context.Background())
	if err != nil {
		t.Fatalf("a save failure must not discard a valid token: %v", err)
	}
	if header != "Bearer minted-token-1" {
		t.Fatalf("header = %q, want the minted token", header)
	}
	if _, statErr := os.Stat(cfg.Path); statErr == nil {
		t.Fatalf("config unexpectedly written to a read-only dir; test is not proving the degrade path")
	}
}

// TestAuthHeader_DryRunDoesNotMint is blocker 5. doInternal resolves the auth
// header BEFORE its dry-run branch so the preview shows what would be sent;
// --dry-run must therefore never dial the token endpoint.
func TestAuthHeader_DryRunDoesNotMint(t *testing.T) {
	clearVerifyEnv(t)
	ts := newTokenServer(t, 3600)
	cfg := expiringConfig(t, ts, time.Now().Add(-time.Minute))
	c := newTestClient(t, cfg)
	c.DryRun = true

	header, err := c.authHeader(context.Background())
	if err != nil {
		t.Fatalf("authHeader under dry-run: %v", err)
	}
	if header != "Bearer stale-token" {
		t.Fatalf("dry-run header = %q, want the cached token", header)
	}
	if got := ts.grantCount(); got != 0 {
		t.Fatalf("--dry-run hit the token endpoint %d times, want 0", got)
	}
}

// TestAuthHeader_VerifyModeDoesNotMint keeps the printing-press verifier
// offline: a verify subprocess must never perform a real token exchange.
func TestAuthHeader_VerifyModeDoesNotMint(t *testing.T) {
	t.Setenv("PRINTING_PRESS_VERIFY", "1")
	t.Setenv("PRINTING_PRESS_VERIFY_LIVE_HTTP", "")
	ts := newTokenServer(t, 3600)
	cfg := expiringConfig(t, ts, time.Now().Add(-time.Minute))
	c := newTestClient(t, cfg)

	header, err := c.authHeader(context.Background())
	if err != nil {
		t.Fatalf("authHeader under verify: %v", err)
	}
	if header != "Bearer stale-token" {
		t.Fatalf("verify header = %q, want the cached token", header)
	}
	if got := ts.grantCount(); got != 0 {
		t.Fatalf("verify mode hit the token endpoint %d times, want 0", got)
	}
}

// TestAuthHeader_VerifyModeMissingTenantDoesNotFail: verify runs against
// fixture configs that have no tenant_id and no live credential. The migration
// error must not turn a verify pass red.
func TestAuthHeader_VerifyModeMissingTenantDoesNotFail(t *testing.T) {
	t.Setenv("PRINTING_PRESS_VERIFY", "1")
	t.Setenv("PRINTING_PRESS_VERIFY_LIVE_HTTP", "")
	cfg := &config.Config{
		BaseURL:      "http://example.test",
		AccessToken:  "expired-token",
		TokenExpiry:  time.Now().Add(-time.Hour),
		ClientID:     "00000000-0000-0000-0000-000000000000",
		ClientSecret: "s3cret",
		Path:         filepath.Join(t.TempDir(), "config.toml"),
	}
	c := newTestClient(t, cfg)

	if _, err := c.authHeader(context.Background()); err != nil {
		t.Fatalf("verify mode must not raise the migration error: %v", err)
	}
}

// TestMintSurfacesTokenEndpointFailure pins that a rotated/revoked client
// secret produces the vendor's own explanation rather than a bare 401 later.
func TestMintSurfacesTokenEndpointFailure(t *testing.T) {
	clearVerifyEnv(t)
	ts := newTokenServer(t, 3600)
	ts.statusOut = http.StatusUnauthorized
	cfg := expiringConfig(t, ts, time.Now().Add(-time.Minute))
	c := newTestClient(t, cfg)

	header, err := c.authHeader(context.Background())
	if err == nil {
		t.Fatalf("expected the mint failure to surface, got header %q", header)
	}
	if !strings.Contains(err.Error(), "invalid_client") {
		t.Fatalf("error should carry the token endpoint's explanation, got: %v", err)
	}
}

// TestMintSurvivesRedirectingTokenEndpoint pins the non-reentrancy guard.
// c.HTTPClient's CheckRedirect (installed in New) re-derives auth on every
// SAME-HOST hop by calling c.authHeader, which takes ccMu. The mint runs while
// HOLDING ccMu, so if the token exchange used c.HTTPClient verbatim a single
// same-host 302 from the token endpoint would deadlock the process forever.
// mintHTTPClient clears CheckRedirect on its copy; this test fails by TIMING
// OUT (not by assertion) if that is ever undone.
func TestMintSurvivesRedirectingTokenEndpoint(t *testing.T) {
	clearVerifyEnv(t)
	var grants int64
	// One server, two paths: the token path 302s to a second path on the SAME
	// host, which is the branch that re-derives the Authorization header.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/hop/") {
			http.Redirect(w, r, "/hop"+r.URL.Path, http.StatusFound)
			return
		}
		n := atomic.AddInt64(&grants, 1)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"access_token":%q,"expires_in":3600}`, fmt.Sprintf("redirected-token-%d", n))
	}))
	t.Cleanup(srv.Close)

	cfg := &config.Config{
		BaseURL:      "http://example.test",
		AccessToken:  "stale-token",
		TokenExpiry:  time.Now().Add(-time.Minute),
		ClientID:     "00000000-0000-0000-0000-000000000000",
		ClientSecret: "s3cret",
		TenantID:     "11111111-1111-1111-1111-111111111111",
		Authority:    srv.URL,
		Path:         filepath.Join(t.TempDir(), "config.toml"),
	}
	c := newTestClient(t, cfg)

	done := make(chan struct{})
	var header string
	var err error
	go func() {
		defer close(done)
		header, err = c.authHeader(context.Background())
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("mint deadlocked on a same-host redirect: CheckRedirect re-entered ccMu")
	}
	if err != nil {
		t.Fatalf("authHeader: %v", err)
	}
	if header != "Bearer redirected-token-1" {
		t.Fatalf("header = %q, want the token from the redirect target", header)
	}
}
