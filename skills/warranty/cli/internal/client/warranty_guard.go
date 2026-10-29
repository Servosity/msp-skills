// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written (handfixes.json warranty-host-pinning): one transport guard on
// every client. It keeps the program on the three vendors' endpoints, routes
// the Apple bearer, and refuses a redirect off the request's origin. Which
// credential may reach which origin is the shared credential kit's job
// (internal/credwire declares it): the guard sits outside the kit, so the kit
// judges every request exactly as it leaves, the Apple bearer included.

package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"warranty-pp-cli/internal/cliutil"
	"warranty-pp-cli/internal/credkit"
)

// Vendor origins. Dell's comes from the configured base URL and token URL.
const (
	LenovoOrigin   = "https://supportapi.lenovo.com:443"
	AppleAPIOrigin = "https://api-business.apple.com:443"
	// AppleOAuthEndpoint is Apple's OAuth token endpoint (the assertion's aud
	// claim is AppleOAuthAudience, a different URL).
	AppleOAuthEndpoint = "https://account.apple.com/auth/oauth2/token"
)

// Credential environment variables for the vendors outside the generated
// Dell config.
const (
	EnvLenovoClientKey  = "WARRANTY_LENOVO_CLIENT_KEY"
	EnvAppleClientID    = "WARRANTY_APPLE_CLIENT_ID"
	EnvAppleKeyID       = "WARRANTY_APPLE_KEY_ID"
	EnvApplePrivateKey  = "WARRANTY_APPLE_PRIVATE_KEY"
	lenovoHeader        = "ClientID"
	appleTokenLifetime  = 50 * time.Minute // Apple tokens last 3600 s
	appleTokenBodyLimit = 64 << 10
	// dellOAuthEndpoint is the generated client's default when no token_url is
	// configured (client.go mintClientCredentialsRaw).
	dellOAuthEndpoint = "https://apigtwb2c.us.dell.com/auth/oauth/v2/token"
)

// ErrVendorNotConfigured marks a request to a vendor whose credential is not
// set, refused before it leaves the machine.
var ErrVendorNotConfigured = errors.New("vendor not configured")

// Kit sources of what this package holds while running. internal/credwire binds
// each to its one vendor's origins, so a token minted for one vendor can never
// leave for another.
const (
	SourceDellMint       = "minted:dell-token"
	sourceAppleMint      = "minted:apple-token"
	sourceAppleAssertion = "minted:apple-assertion"
	sourceAppleKey       = "minted:apple-key"
	// SourceAppleClientID holds the unique part of the Apple client ID.
	SourceAppleClientID = "minted:apple-client-id"
)

// KitClient is hc with its requests sent through the credential kit's outbound
// guard, unless the vendor guard already sends them through it.
func KitClient(hc *http.Client) *http.Client {
	if hc == nil {
		hc = &http.Client{}
	}
	if _, guarded := hc.Transport.(*guardTransport); guarded {
		return hc
	}
	kit := *hc
	kit.Transport = OutboundGuard(hc.Transport)
	return &kit
}

// ErrHostNotAllowed marks a request or redirect to an origin that is not a
// configured vendor endpoint.
var ErrHostNotAllowed = errors.New("refused: not a configured vendor endpoint")

// Origin returns scheme://host:port with the default port made explicit, so
// https://x and https://x:443 compare equal. Empty for an unparseable URL.
func Origin(u *url.URL) string {
	if u == nil || u.Hostname() == "" {
		return ""
	}
	scheme := strings.ToLower(u.Scheme)
	port := u.Port()
	if port == "" {
		switch scheme {
		case "https":
			port = "443"
		case "http":
			port = "80"
		}
	}
	return scheme + "://" + strings.ToLower(u.Hostname()) + ":" + port
}

func originOf(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	return Origin(u)
}

// AppleCredentials is one Apple Business API account.
type AppleCredentials struct {
	ClientID   string
	KeyID      string
	PrivateKey string
}

// AppleCredentialsFromEnv reads the Apple variables and names the unset ones.
func AppleCredentialsFromEnv() (AppleCredentials, []string) {
	c := AppleCredentials{
		ClientID:   strings.TrimSpace(cliutil.EnvOverride(EnvAppleClientID)),
		KeyID:      strings.TrimSpace(cliutil.EnvOverride(EnvAppleKeyID)),
		PrivateKey: cliutil.EnvOverride(EnvApplePrivateKey),
	}
	var unset []string
	if c.ClientID == "" {
		unset = append(unset, EnvAppleClientID)
	}
	if c.KeyID == "" {
		unset = append(unset, EnvAppleKeyID)
	}
	if strings.TrimSpace(c.PrivateKey) == "" {
		unset = append(unset, EnvApplePrivateKey)
	}
	return c, unset
}

// LenovoKeyFromEnv returns the Lenovo ClientID token, or "".
func LenovoKeyFromEnv() string {
	return strings.TrimSpace(cliutil.EnvOverride(EnvLenovoClientKey))
}

// appleTokenSource mints and caches one Apple access token per process.
type appleTokenSource struct {
	mu       sync.Mutex
	creds    AppleCredentials
	tokenURL string
	http     *http.Client
	token    string
	expiry   time.Time
}

var (
	appleSourcesMu sync.Mutex
	appleSources   = map[string]*appleTokenSource{}
)

// sharedAppleSource returns the process's token source for these credentials,
// so every client (CLI, MCP, tier copies) reuses one cached token.
func sharedAppleSource(creds AppleCredentials, base http.RoundTripper) *appleTokenSource {
	key := creds.ClientID + "\x00" + creds.KeyID
	appleSourcesMu.Lock()
	defer appleSourcesMu.Unlock()
	if s, ok := appleSources[key]; ok {
		return s
	}
	s := &appleTokenSource{
		creds:    creds,
		tokenURL: AppleOAuthEndpoint,
		http: &http.Client{
			Timeout:   30 * time.Second,
			Transport: base,
			// The token request carries a signed assertion: never follow a
			// redirect anywhere.
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}
	appleSources[key] = s
	return s
}

func (s *appleTokenSource) Token(ctx context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.token != "" && time.Now().Before(s.expiry) {
		return s.token, nil
	}
	form := url.Values{"grant_type": {"client_credentials"}, "scope": {"business.api"}}
	if err := AddAppleClientAssertion(form, s.creds.ClientID, s.creds.PrivateKey, s.creds.KeyID); err != nil {
		return "", fmt.Errorf("%w: Apple Business: %v", ErrVendorNotConfigured, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.tokenURL, strings.NewReader(TokenForm(form)))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := s.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("Apple Business token request: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, appleTokenBodyLimit))
	if err != nil {
		return "", fmt.Errorf("Apple Business token request: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Apple Business token request: HTTP %d: %s", resp.StatusCode, cliutil.SanitizeErrorBody(string(body)))
	}
	var tr struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &tr); err != nil || strings.TrimSpace(tr.AccessToken) == "" {
		return "", errors.New("Apple Business token response has no access_token")
	}
	credkit.Hold(sourceAppleMint, tr.AccessToken) // bound to Apple's API origin, before anything can carry it
	life := appleTokenLifetime
	if tr.ExpiresIn > 120 && time.Duration(tr.ExpiresIn-60)*time.Second < life {
		life = time.Duration(tr.ExpiresIn-60) * time.Second
	}
	s.token, s.expiry = tr.AccessToken, time.Now().Add(life)
	return s.token, nil
}

// guardTransport enforces the vendor origin allowlist and adds the Apple
// bearer token. It sits outside the generated transport, so it sees every
// request the client sends, the Dell token mint included.
type guardTransport struct {
	base    http.RoundTripper
	allowed map[string]bool
	apple   func() (*appleTokenSource, []string)
}

func (t *guardTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	origin := Origin(req.URL)
	if !t.allowed[origin] {
		return nil, fmt.Errorf("%w: %s://%s", ErrHostNotAllowed, req.URL.Scheme, req.URL.Host)
	}
	req = pinCredentialHeaders(req, origin)
	switch origin {
	case LenovoOrigin:
		if strings.TrimSpace(req.Header.Get(lenovoHeader)) == "" {
			return nil, fmt.Errorf("%w: Lenovo: set %s (a ClientID token from your Lenovo account manager)", ErrVendorNotConfigured, EnvLenovoClientKey)
		}
	case AppleAPIOrigin:
		src, unset := t.apple()
		if src == nil {
			return nil, fmt.Errorf("%w: Apple Business: set %s", ErrVendorNotConfigured, strings.Join(unset, ", "))
		}
		tok, err := src.Token(req.Context())
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	return t.base.RoundTrip(req)
}

// ownCredentialHeader is the one header that may carry a credential to an
// origin: ClientID to Lenovo, Authorization to Apple (the minted bearer) and
// to Dell (Basic on the token request, Bearer on the API).
func ownCredentialHeader(origin string) string {
	if origin == LenovoOrigin {
		return http.CanonicalHeaderKey(lenovoHeader)
	}
	return "Authorization"
}

// pinCredentialHeaders returns a copy of req without any header whose name
// marks a credential, unless it is the destination's own credential header:
// a configured [headers] entry meant for Dell is not offered to the others.
// A held value under any other header name is the kit's refusal to make.
func pinCredentialHeaders(req *http.Request, origin string) *http.Request {
	own := ownCredentialHeader(origin)
	out := req.Clone(req.Context())
	for name := range out.Header {
		if http.CanonicalHeaderKey(name) == own {
			continue
		}
		if credentialLikeName(name) {
			out.Header.Del(name)
		}
	}
	return out
}

// credentialLikeName reports whether a header name looks like it carries a
// credential.
func credentialLikeName(name string) bool {
	k := strings.NewReplacer("-", "", "_", "", ".", "", " ", "").Replace(strings.ToLower(name))
	for _, w := range []string{"secret", "token", "key", "password", "passwd", "auth", "clientid", "cookie", "signature", "session", "credential"} {
		if strings.Contains(k, w) {
			return true
		}
	}
	return false
}

// InstallGuard wraps c's transport with the vendor guard, refuses redirects
// that leave the request's origin, and turns the on-disk response cache off
// (Apple responses are scoped by an account the cache key cannot see).
// Idempotent.
func InstallGuard(c *Client) {
	if c == nil || c.HTTPClient == nil {
		return
	}
	c.NoCache = true
	if _, done := c.HTTPClient.Transport.(*guardTransport); done {
		return
	}
	allowed := map[string]bool{LenovoOrigin: true, AppleAPIOrigin: true}
	if o := originOf(c.BaseURL); o != "" {
		allowed[o] = true
	}
	tokenURL := dellOAuthEndpoint
	if c.Config != nil && strings.TrimSpace(c.Config.TokenURL) != "" {
		tokenURL = strings.TrimSpace(c.Config.TokenURL)
	}
	if u, err := url.Parse(tokenURL); err == nil && Origin(u) != "" {
		allowed[Origin(u)] = true
	}
	if cliutil.IsVerifyEnv() {
		// The verify harness's mock answers only at the configured base URL
		// (every tier routes there under verify, see baseURLForRequest):
		// never dial a real vendor or token endpoint from it. The generated
		// client's verify token has no expiry and would be re-minted against
		// the real token URL on the next request; give it one.
		allowed = map[string]bool{originOf(c.BaseURL): true}
		if c.Config != nil && c.Config.AccessToken == "" {
			c.Config.AccessToken = "mock-token-for-testing"
			c.Config.TokenExpiry = time.Now().Add(time.Hour)
		}
	}
	base := c.HTTPClient.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	base = OutboundGuard(base)
	c.HTTPClient.Transport = &guardTransport{
		base:    base,
		allowed: allowed,
		apple: func() (*appleTokenSource, []string) {
			creds, unset := AppleCredentialsFromEnv()
			if len(unset) > 0 {
				return nil, unset
			}
			return sharedAppleSource(creds, base), nil
		},
	}
	prev := c.HTTPClient.CheckRedirect
	c.HTTPClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) > 0 && Origin(req.URL) != Origin(via[0].URL) {
			return fmt.Errorf("%w: redirect to %s://%s", ErrHostNotAllowed, req.URL.Scheme, req.URL.Host)
		}
		if prev != nil {
			return prev(req, via)
		}
		return nil
	}
}

// TokenForm encodes an OAuth token form with grant_type first. The kit also reads
// a body as key=value lines, so with a credential field first (client_id and
// client_assertion sort first) it would hold the rest of the body as that field's
// value, and "client_credentials" and "business.api" in that value would withhold
// the words credentials and business from all later output.
// Hand-written (handfixes.json warranty-apple-auth).
func TokenForm(form url.Values) string {
	rest := url.Values{}
	for k, v := range form {
		if k != "grant_type" {
			rest[k] = v
		}
	}
	out := "grant_type=" + url.QueryEscape(form.Get("grant_type"))
	if len(rest) > 0 {
		out += "&" + rest.Encode()
	}
	return out
}

// HoldAppleCredentials holds what WARRANTY_APPLE_CLIENT_ID and
// WARRANTY_APPLE_PRIVATE_KEY name, before any output. Neither is a SecretEnv: the
// kit would hold the private key's value as written, usually a file path sharing
// its directories with every other path this program prints or stores, and the
// client ID whole, whose "BUSINESSAPI." would withhold the word Business, which
// vendors' own coverage names carry.
func HoldAppleCredentials() {
	if raw := os.Getenv(EnvApplePrivateKey); strings.TrimSpace(raw) != "" {
		credkit.Hold(sourceAppleKey, appleKeyHeld(raw)...)
	}
	if id := strings.TrimSpace(os.Getenv(EnvAppleClientID)); id != "" {
		_, suffix, ok := strings.Cut(id, ".")
		if !ok || suffix == "" {
			suffix = id
		}
		credkit.Hold(SourceAppleClientID, suffix)
	}
}

// warrantyGuardRefusal reports a guard refusal: never retried, because the
// same request would be refused again.
func warrantyGuardRefusal(err error) bool {
	return errors.Is(err, ErrHostNotAllowed) || errors.Is(err, ErrVendorNotConfigured) || errors.Is(err, credkit.ErrWithheld)
}
