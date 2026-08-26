// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored regression suite for the two P1 defects an adversarial review
// found in the client-credentials re-mint, plus the mint-storm and
// secret-in-error-body follow-ons.
//
//  1. The mint cleared CheckRedirect to dodge a ccMu re-entrancy deadlock.
//     Clearing it does not disable redirects - it restores Go's DEFAULT policy,
//     which follows up to 10 hops and REPLAYS THE POST BODY on 307/308. That
//     body is the client-credentials form carrying client_secret.
//  2. A failed mint returned its error straight to the caller, so any transient
//     token-endpoint failure during the 5-minute safety skew (about 8% of every
//     token lifetime) broke a CLI whose cached token was still perfectly valid.
//
// Every gate below is proved in BOTH directions: it fires on the broken shape
// and stays silent on the healthy one.

package client

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"cipp-pp-cli/internal/config"
)

// A deliberately low-entropy, stopword-bearing literal: the secret scanner in
// CI flags high-entropy strings next to the word "secret", and a test fixture
// must never look like a real credential.
const testClientSecret = "not-a-real-secret-example"

// recordingHost stands in for the host a hostile or misconfigured token
// endpoint redirects to. It records every request body it is handed, which is
// the only thing that matters: if the client_credentials form reaches it, the
// secret has leaked whether or not the CLI accepts the reply.
type recordingHost struct {
	srv    *httptest.Server
	mu     sync.Mutex
	bodies []string
}

func newRecordingHost(t *testing.T) *recordingHost {
	t.Helper()
	h := &recordingHost{}
	h.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		h.mu.Lock()
		h.bodies = append(h.bodies, string(body))
		h.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"access_token":"attacker-supplied-token","expires_in":3600}`)
	}))
	t.Cleanup(h.srv.Close)
	return h
}

func (h *recordingHost) hits() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.bodies)
}

func (h *recordingHost) sawSecret() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, b := range h.bodies {
		if strings.Contains(b, testClientSecret) {
			return true
		}
	}
	return false
}

// newRedirector returns a server whose every path answers `status` with a
// Location pointing at target. 307/308 are the statuses Go replays the body on.
func newRedirector(t *testing.T, target string, status int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target, status)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func credentialForm() string {
	form := url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {"00000000-0000-0000-0000-000000000000"},
		"client_secret": {testClientSecret},
	}
	return form.Encode()
}

// --- P1 #1: the redirect policy -----------------------------------------

// TestTokenExchangeRedirectPolicy is the unit-level both-directions proof. The
// same-origin rows must be permitted (a token endpoint legitimately 302s
// through its own front door) and every cross-origin row refused.
func TestTokenExchangeRedirectPolicy(t *testing.T) {
	mk := func(raw string) *http.Request {
		u, err := url.Parse(raw)
		if err != nil {
			t.Fatalf("parse %q: %v", raw, err)
		}
		return &http.Request{URL: u}
	}
	cases := []struct {
		name    string
		from    string
		to      string
		allowed bool
	}{
		{"identical url", "https://login.example.com/t/token", "https://login.example.com/t/token", true},
		{"same origin, different path", "https://login.example.com/t/token", "https://login.example.com/hop/t/token", true},
		{"same host, http to https upgrade", "http://login.example.com/token", "https://login.example.com/token", true},
		{"same host, case-insensitive", "https://Login.Example.com/token", "https://login.example.COM/token", true},
		{"different host", "https://login.example.com/token", "https://evil.example.net/token", false},
		{"subdomain of the same domain", "https://login.example.com/token", "https://evil.login.example.com/token", false},
		{"same host, different port", "http://127.0.0.1:8080/token", "http://127.0.0.1:9090/token", false},
		{"https downgraded to http", "https://login.example.com/token", "http://login.example.com/token", false},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			err := TokenExchangeRedirectPolicy(mk(tc.to), []*http.Request{mk(tc.from)})
			if tc.allowed && err != nil {
				t.Fatalf("same-origin hop was refused: %v", err)
			}
			if !tc.allowed {
				if err == nil {
					t.Fatalf("cross-origin hop %s -> %s was ALLOWED; the client_credentials form would be replayed there", tc.from, tc.to)
				}
				if !strings.Contains(err.Error(), "client_secret") {
					t.Fatalf("refusal must say why; got: %v", err)
				}
			}
		})
	}

	// The hop cap survives: Go's own default stops at 10 and so must this one,
	// or a same-origin redirect loop spins forever.
	via := make([]*http.Request, 10)
	for i := range via {
		via[i] = mk("https://login.example.com/token")
	}
	if err := TokenExchangeRedirectPolicy(mk("https://login.example.com/token"), via); err == nil {
		t.Fatal("policy followed an 11th hop; a redirect loop would never terminate")
	}
}

// TestDefaultRedirectPolicyLeaksTheSecretAndOursDoesNot is the fires-on-broken
// half, run against real sockets. It first reproduces the defect exactly as it
// shipped - CheckRedirect nil, which IS Go's default policy - and proves the
// client_secret lands on the other host. It then swaps in
// TokenExchangeRedirectPolicy and proves the same request never reaches it.
func TestDefaultRedirectPolicyLeaksTheSecretAndOursDoesNot(t *testing.T) {
	post := func(client *http.Client, tokenURL string) {
		req, err := http.NewRequest(http.MethodPost, tokenURL, strings.NewReader(credentialForm()))
		if err != nil {
			t.Fatalf("new request: %v", err)
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		resp, err := client.Do(req)
		if err == nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
		}
	}

	t.Run("broken: CheckRedirect nil is Go's body-replaying default", func(t *testing.T) {
		attacker := newRecordingHost(t)
		redirector := newRedirector(t, attacker.srv.URL+"/token", http.StatusTemporaryRedirect)
		post(&http.Client{Timeout: 5 * time.Second, CheckRedirect: nil}, redirector.URL+"/tenant/oauth2/v2.0/token")
		if attacker.hits() == 0 {
			t.Fatal("test is not exercising the leak: the 307 was not followed at all")
		}
		if !attacker.sawSecret() {
			t.Fatalf("test is not exercising the leak: the redirect target got %d request(s) but no client_secret", attacker.hits())
		}
	})

	t.Run("healthy: TokenExchangeRedirectPolicy refuses the hop", func(t *testing.T) {
		attacker := newRecordingHost(t)
		redirector := newRedirector(t, attacker.srv.URL+"/token", http.StatusTemporaryRedirect)
		post(&http.Client{Timeout: 5 * time.Second, CheckRedirect: TokenExchangeRedirectPolicy}, redirector.URL+"/tenant/oauth2/v2.0/token")
		if got := attacker.hits(); got != 0 {
			t.Fatalf("the cross-host redirect target received %d request(s), want 0", got)
		}
	})
}

// TestMintRefusesCrossHostRedirect is P1 #1 end to end, through the real mint.
// A token endpoint answering 307 to a different host must not receive the form
// body, and the token it offers must not be accepted or cached.
func TestMintRefusesCrossHostRedirect(t *testing.T) {
	clearVerifyEnv(t)
	attacker := newRecordingHost(t)
	redirector := newRedirector(t, attacker.srv.URL+"/token", http.StatusTemporaryRedirect)

	cfg := &config.Config{
		BaseURL:      "http://example.test",
		AccessToken:  "stale-token",
		TokenExpiry:  time.Now().Add(-time.Minute),
		ClientID:     "00000000-0000-0000-0000-000000000000",
		ClientSecret: testClientSecret,
		TenantID:     "11111111-1111-1111-1111-111111111111",
		Authority:    redirector.URL,
		Path:         filepath.Join(t.TempDir(), "config.toml"),
	}
	c := newTestClient(t, cfg)

	header, err := c.authHeader(context.Background())

	// 1. The secret never left for the other host.
	if got := attacker.hits(); got != 0 {
		t.Fatalf("the redirect target received %d request(s); the client_credentials form was replayed cross-host", got)
	}
	if attacker.sawSecret() {
		t.Fatal("client_secret reached the redirect target")
	}
	// 2. Its token was not accepted.
	if err == nil {
		t.Fatalf("cross-host redirect was followed and accepted; header = %q", header)
	}
	if header != "" {
		t.Fatalf("header = %q, want empty on a refused mint", header)
	}
	if strings.Contains(header, "attacker-supplied-token") || cfg.AccessToken == "attacker-supplied-token" {
		t.Fatalf("the redirect target's token was installed: header=%q access_token=%q", header, cfg.AccessToken)
	}
	// 3. Nothing was cached: the pre-existing token is untouched.
	if cfg.AccessToken != "stale-token" {
		t.Fatalf("access_token = %q, want the untouched cached value", cfg.AccessToken)
	}
	// 4. The error explains itself without leaking the secret.
	msg := err.Error()
	for _, want := range []string{"refusing to resend", "client_secret", redirector.URL, attacker.srv.URL} {
		if !strings.Contains(msg, want) {
			t.Fatalf("error must name %q; got:\n%s", want, msg)
		}
	}
	if strings.Contains(msg, testClientSecret) {
		t.Fatalf("error leaked the client secret:\n%s", msg)
	}
}

// TestMintFollowsSameHostRedirectWithBody is the stays-silent-on-healthy half of
// the same gate, and it is the case that must keep working: a token endpoint
// that 307s within its OWN origin still gets the form and still yields a token.
// A blanket "never redirect" fix would fail here.
func TestMintFollowsSameHostRedirectWithBody(t *testing.T) {
	clearVerifyEnv(t)
	var mu sync.Mutex
	var hopBodies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/hop/") {
			http.Redirect(w, r, "/hop"+r.URL.Path, http.StatusTemporaryRedirect)
			return
		}
		body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		mu.Lock()
		hopBodies = append(hopBodies, string(body))
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"access_token":"same-host-token","expires_in":3600}`)
	}))
	t.Cleanup(srv.Close)

	cfg := &config.Config{
		BaseURL:      "http://example.test",
		AccessToken:  "stale-token",
		TokenExpiry:  time.Now().Add(-time.Minute),
		ClientID:     "00000000-0000-0000-0000-000000000000",
		ClientSecret: testClientSecret,
		TenantID:     "11111111-1111-1111-1111-111111111111",
		Authority:    srv.URL,
		Path:         filepath.Join(t.TempDir(), "config.toml"),
	}
	c := newTestClient(t, cfg)

	header, err := c.authHeader(context.Background())
	if err != nil {
		t.Fatalf("a same-origin 307 must still complete the exchange: %v", err)
	}
	if header != "Bearer same-host-token" {
		t.Fatalf("header = %q, want the token from the same-host hop", header)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(hopBodies) != 1 {
		t.Fatalf("redirect target saw %d bodies, want 1", len(hopBodies))
	}
	if !strings.Contains(hopBodies[0], "grant_type=client_credentials") {
		t.Fatalf("the 307 did not replay the form to the same origin: %q", hopBodies[0])
	}
}

// --- P1 #2: a failed mint must not discard a still-valid token ------------

// TestAuthHeader_MintFailureKeepsStillValidToken is P1 #2. The token is inside
// the 5-minute safety skew but has NOT expired, so the request it is about to
// authorize will still succeed. A 503 from the token endpoint must warn, not
// break the install minutes early.
func TestAuthHeader_MintFailureKeepsStillValidToken(t *testing.T) {
	clearVerifyEnv(t)
	ts := newTokenServer(t, 3600)
	ts.statusOut = http.StatusServiceUnavailable
	cfg := expiringConfig(t, ts, time.Now().Add(2*time.Minute))
	cfg.AccessToken = "still-valid-token"
	c := newTestClient(t, cfg)

	var header string
	var err error
	stderr := captureStderr(t, func() {
		header, err = c.authHeader(context.Background())
	})

	if err != nil {
		t.Fatalf("a transient token-endpoint failure broke a working install: %v", err)
	}
	if header != "Bearer still-valid-token" {
		t.Fatalf("header = %q, want the still-valid cached token", header)
	}
	if got := ts.grantCount(); got != 1 {
		t.Fatalf("token endpoint saw %d attempts, want exactly 1", got)
	}
	for _, want := range []string{"refreshing the CIPP access token failed", "still valid until"} {
		if !strings.Contains(stderr, want) {
			t.Fatalf("the degrade must be loud on stderr and name %q; got:\n%s", want, stderr)
		}
	}
	if strings.Contains(stderr, testClientSecret) {
		t.Fatalf("the warning leaked the client secret:\n%s", stderr)
	}
}

// TestAuthHeader_MintFailureOnDeadTokenStillFailsHard is the other direction.
// Degrading is only correct while the cached credential still works; once it is
// genuinely expired there is nothing to ride, and swallowing the error would
// reproduce the unexplained 401 this whole change exists to remove.
func TestAuthHeader_MintFailureOnDeadTokenStillFailsHard(t *testing.T) {
	clearVerifyEnv(t)
	ts := newTokenServer(t, 3600)
	ts.statusOut = http.StatusServiceUnavailable
	cfg := expiringConfig(t, ts, time.Now().Add(-time.Minute))
	c := newTestClient(t, cfg)

	var header string
	var err error
	_ = captureStderr(t, func() { header, err = c.authHeader(context.Background()) })

	if err == nil {
		t.Fatalf("an expired token plus a failing mint must fail hard; got header %q", header)
	}
	if header != "" {
		t.Fatalf("a dead token leaked through the error path: %q", header)
	}
	if !strings.Contains(err.Error(), "invalid_client") {
		t.Fatalf("error should carry the token endpoint's own explanation: %v", err)
	}
}

// TestAuthHeader_MintFailureOnZeroExpiryFailsHard covers the third state. A
// zero expiry means "unknown", so the client cannot claim the token still
// works; only an expiry it can compare against justifies degrading.
func TestAuthHeader_MintFailureOnZeroExpiryFailsHard(t *testing.T) {
	clearVerifyEnv(t)
	ts := newTokenServer(t, 3600)
	ts.statusOut = http.StatusServiceUnavailable
	authority, tenant := ts.tenantFor()
	cfg := &config.Config{
		BaseURL:      "http://example.test",
		ClientID:     "00000000-0000-0000-0000-000000000000",
		ClientSecret: testClientSecret,
		TenantID:     tenant,
		Authority:    authority,
		Path:         filepath.Join(t.TempDir(), "config.toml"),
	}
	c := newTestClient(t, cfg)

	var err error
	_ = captureStderr(t, func() { _, err = c.authHeader(context.Background()) })
	if err == nil {
		t.Fatal("no cached token at all plus a failing mint must fail hard")
	}
}

// TestAuthHeader_MintFailureDoesNotStormTheTokenEndpoint pins the cost of the
// degrade. Without a cooldown every request in a fan-out would re-attempt the
// failing exchange while HOLDING ccMu for up to tokenRequestTimeout, so a
// degraded token endpoint would be slower than a dead one.
func TestAuthHeader_MintFailureDoesNotStormTheTokenEndpoint(t *testing.T) {
	clearVerifyEnv(t)
	ts := newTokenServer(t, 3600)
	ts.statusOut = http.StatusServiceUnavailable
	cfg := expiringConfig(t, ts, time.Now().Add(2*time.Minute))
	cfg.AccessToken = "still-valid-token"
	c := newTestClient(t, cfg)

	_ = captureStderr(t, func() {
		for i := 0; i < 25; i++ {
			header, err := c.authHeader(context.Background())
			if err != nil {
				t.Errorf("call %d: %v", i, err)
				return
			}
			if header != "Bearer still-valid-token" {
				t.Errorf("call %d header = %q", i, header)
				return
			}
		}
	})
	if t.Failed() {
		return
	}
	if got := ts.grantCount(); got != 1 {
		t.Fatalf("a failing token endpoint was hit %d times over 25 requests, want 1 (cooldown)", got)
	}
}

// --- P2: the mint storm from an unclamped skew ---------------------------

// TestEffectiveTokenSkewClampsToHalfTheGrantedLifetime proves the clamp in both
// directions: a normal Azure-AD-shaped lifetime keeps the full 5 minutes, and
// anything shorter than 10 minutes is halved so a fresh token is never inside
// its own skew.
func TestEffectiveTokenSkewClampsToHalfTheGrantedLifetime(t *testing.T) {
	cases := []struct {
		name     string
		lifetime int
		want     time.Duration
	}{
		{"unknown lifetime keeps the default", 0, 5 * time.Minute},
		{"negative lifetime keeps the default", -1, 5 * time.Minute},
		{"3600s Azure AD token keeps the default", 3600, 5 * time.Minute},
		{"600s token is exactly at the boundary", 600, 5 * time.Minute},
		{"120s token halves to 60s", 120, 60 * time.Second},
		{"60s token halves to 30s", 60, 30 * time.Second},
		{"nil config keeps the default", -999, 5 * time.Minute},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			var cfg *config.Config
			if tc.lifetime != -999 {
				cfg = &config.Config{TokenLifetime: tc.lifetime}
			}
			if got := effectiveTokenSkew(cfg); got != tc.want {
				t.Fatalf("effectiveTokenSkew = %s, want %s", got, tc.want)
			}
		})
	}
}

// TestNeedsClientCredentialsMint_ShortLivedToken is the both-directions proof at
// the predicate. A 120-second token with 118 seconds left is FRESH; the
// unclamped 5-minute skew called it stale, which is the storm.
func TestNeedsClientCredentialsMint_ShortLivedToken(t *testing.T) {
	base := func(lifetime int, remaining time.Duration) *config.Config {
		return &config.Config{
			ClientID:      "id",
			ClientSecret:  "sec",
			AccessToken:   "tok",
			TokenExpiry:   time.Now().Add(remaining),
			TokenLifetime: lifetime,
		}
	}
	if needsClientCredentialsMint(base(120, 118*time.Second)) {
		t.Fatal("a token 2 seconds old out of a 120-second lifetime was called stale: every request would re-mint")
	}
	if !needsClientCredentialsMint(base(120, 30*time.Second)) {
		t.Fatal("a 120-second token with 30 seconds left is inside its clamped 60-second skew and must re-mint")
	}
	// The clamp must not weaken the normal case.
	if !needsClientCredentialsMint(base(3600, 2*time.Minute)) {
		t.Fatal("a 3600-second token with 2 minutes left must still re-mint")
	}
	if needsClientCredentialsMint(base(3600, 30*time.Minute)) {
		t.Fatal("a 3600-second token with 30 minutes left must not re-mint")
	}
	// A config that predates token_lifetime keeps the old behaviour exactly.
	if !needsClientCredentialsMint(base(0, 2*time.Minute)) {
		t.Fatal("an unknown lifetime must keep the 5-minute default skew")
	}
}

// TestAuthHeader_ShortLivedTokenDoesNotStorm drives the clamp end to end
// against a token endpoint that grants 120-second tokens.
func TestAuthHeader_ShortLivedTokenDoesNotStorm(t *testing.T) {
	clearVerifyEnv(t)
	ts := newTokenServer(t, 120)
	cfg := expiringConfig(t, ts, time.Now().Add(-time.Minute))
	c := newTestClient(t, cfg)

	for i := 0; i < 25; i++ {
		header, err := c.authHeader(context.Background())
		if err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
		if header != "Bearer minted-token-1" {
			t.Fatalf("call %d header = %q, want the single minted token", i, header)
		}
	}
	if got := ts.grantCount(); got != 1 {
		t.Fatalf("a 120-second token produced %d grants over 25 requests, want 1", got)
	}
	if cfg.TokenLifetime != 120 {
		t.Fatalf("granted lifetime not recorded: TokenLifetime = %d", cfg.TokenLifetime)
	}
}

// --- P2: the mint's HTTP-error path must not print the secret -------------

// TestMintErrorBodyMasksClientSecret pins the last leak. cliutil's
// SanitizeErrorBody knows only generic token shapes, so a token endpoint that
// echoes the submitted form back in its error body printed client_secret
// verbatim into the operator's terminal and into any log scraping stderr.
func TestMintErrorBodyMasksClientSecret(t *testing.T) {
	clearVerifyEnv(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		w.WriteHeader(http.StatusBadRequest)
		// A real AAD-shaped 400 that echoes the request it rejected.
		fmt.Fprintf(w, `{"error":"invalid_request","error_description":"rejected request: %s"}`, string(body))
	}))
	t.Cleanup(srv.Close)

	cfg := &config.Config{
		BaseURL:      "http://example.test",
		AccessToken:  "expired-token",
		TokenExpiry:  time.Now().Add(-time.Minute),
		ClientID:     "00000000-0000-0000-0000-000000000000",
		ClientSecret: testClientSecret,
		TenantID:     "11111111-1111-1111-1111-111111111111",
		Authority:    srv.URL,
		Path:         filepath.Join(t.TempDir(), "config.toml"),
	}
	c := newTestClient(t, cfg)

	var err error
	_ = captureStderr(t, func() { _, err = c.authHeader(context.Background()) })
	if err == nil {
		t.Fatal("a 400 from the token endpoint must surface as an error")
	}
	msg := err.Error()
	// Fires on broken: the raw secret, and its form-encoded spelling, are gone.
	if strings.Contains(msg, testClientSecret) {
		t.Fatalf("client_secret printed verbatim in the mint error:\n%s", msg)
	}
	if esc := url.QueryEscape(testClientSecret); esc != testClientSecret && strings.Contains(msg, esc) {
		t.Fatalf("url-encoded client_secret printed in the mint error:\n%s", msg)
	}
	// Stays useful on healthy: the vendor's explanation still reaches the operator.
	if !strings.Contains(msg, "invalid_request") {
		t.Fatalf("masking swallowed the vendor's explanation:\n%s", msg)
	}
	if !strings.Contains(msg, "****mple") {
		t.Fatalf("the secret should be replaced by its masked form, not deleted:\n%s", msg)
	}
}
