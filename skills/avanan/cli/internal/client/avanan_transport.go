// Copyright 2026 geekbrownbear and contributors. Licensed under Apache-2.0. See LICENSE.

// Avanan request signing and session-token management.
//
// This lives at the http.RoundTripper layer rather than in a per-command
// helper because Avanan's legacy scheme signs the *final* request URL,
// including the encoded query string. Anything that builds the signature
// earlier than the transport risks signing a different string than the one
// that goes on the wire, which the server rejects with a 401 that looks
// exactly like a bad credential.
//
// Installed for every client via registerClientHook (see
// internal/cli/avanan_client_hook.go), so generated endpoint commands and
// hand-written commands share one code path.

package client

import (
	"avanan-pp-cli/internal/avanansig"
	"avanan-pp-cli/internal/cliutil"
	"avanan-pp-cli/internal/config"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// legacyTokenTTL is how long a legacy handshake token is trusted. The vendor
// documents a one-hour lifetime; we refresh early so a request never departs
// with a token that expires in flight.
const legacyTokenTTL = 55 * time.Minute

// tokenRefreshSkew is subtracted from a server-advertised expiry for the same
// reason.
const tokenRefreshSkew = 60 * time.Second

// AvananTransport signs outbound requests and manages the session token.
type AvananTransport struct {
	base http.RoundTripper
	cfg  *config.Config

	mu     sync.Mutex
	token  string
	expiry time.Time
}

// InstallAvananAuth wraps a client's transport with Avanan request signing.
// Registered as a client hook so it applies to every constructed client.
func InstallAvananAuth(c *Client) error {
	if c == nil || c.HTTPClient == nil {
		return errors.New("install avanan auth: nil client")
	}
	if _, already := c.HTTPClient.Transport.(*AvananTransport); already {
		return nil
	}
	base := c.HTTPClient.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	t := &AvananTransport{base: base, cfg: c.Config}
	if c.Config != nil && c.Config.AvananToken != "" {
		t.token = c.Config.AvananToken
		// Prefer the expiry `auth login` persisted. Fabricating now+TTL for a
		// token stored hours ago makes a token-only config (no secret to
		// re-mint with, so the 401 retry is skipped) fail every command with a
		// bare 401 that looks like a bad credential.
		if exp := c.Config.TokenExpiry; !exp.IsZero() {
			t.expiry = exp
		} else {
			t.expiry = time.Now().Add(legacyTokenTTL)
		}
	}
	c.HTTPClient.Transport = t
	return nil
}

// credentialsConfigured reports whether the app-id/secret pair needed to mint
// a token is present.
func (t *AvananTransport) credentialsConfigured() bool {
	return t.cfg != nil && t.cfg.AvananAppId != "" && t.cfg.ClientSecret != ""
}

// configuredHost returns the host of the configured base URL - the only host
// this transport will authenticate to.
func (t *AvananTransport) configuredHost() (string, error) {
	if t.cfg == nil || strings.TrimSpace(t.cfg.BaseURL) == "" {
		return "", errors.New("avanan: no base URL configured, so the host a credential would be sent to cannot be verified; set AVANAN_BASE_URL or pass --base-url")
	}
	u, err := url.Parse(strings.TrimSpace(t.cfg.BaseURL))
	if err != nil || u.Host == "" {
		return "", fmt.Errorf("avanan: configured base URL %q is not a usable absolute URL", t.cfg.BaseURL)
	}
	return strings.ToLower(u.Host), nil
}

// authorizeHost refuses any request bound for a host other than the configured
// one.
//
// This exists because RoundTrip runs once per redirect hop, after
// CheckRedirect. CheckRedirect deletes x-av-token on a cross-host hop (Go
// strips standard auth headers but not custom ones), and without this gate the
// transport re-signed and re-stamped the very next hop, undoing that defense
// one instruction later. The Infinity branch made it worse than a token leak:
// the handshake endpoint was derived from the redirect target, so a 302 to an
// attacker-controlled host got the raw client secret POSTed to it.
//
// Refusing outright, rather than proceeding unauthenticated, is deliberate.
// Avanan serves the legacy *.avanan.net farms and the Infinity Portal as
// separate endpoints; migrating between them is a support-ticket operation, not
// a redirect, and no cross-host 3xx appears anywhere in the vendor reference or
// in the two shipping clients this connector was cross-checked against. So a
// cross-host hop here is an open redirect or a misconfiguration, and the
// operator is better served by a named host and a next step than by an
// unauthenticated retry surfacing as an unexplained 401 from a stranger's
// server. If Avanan ever does introduce a legitimate cross-host redirect, this
// fails loudly with the target host in hand rather than silently handing that
// host a credential.
func (t *AvananTransport) authorizeHost(u *url.URL) error {
	want, err := t.configuredHost()
	if err != nil {
		return err
	}
	got := strings.ToLower(u.Host)
	if got == want {
		return nil
	}
	return fmt.Errorf(
		"avanan: refusing to send credentials to %s; this client is configured for %s\n"+
			"a cross-host redirect is not part of the Avanan API - treat this as an open redirect or a misconfigured base URL\n"+
			"if the tenant genuinely moved, point the CLI at the new host with --base-url or AVANAN_BASE_URL",
		got, want)
}

// RoundTrip signs and dispatches a single request.
func (t *AvananTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// A RoundTripper must not mutate the request it is handed.
	r := req.Clone(req.Context())

	// Gate before anything reads the host: the scheme decision, the handshake
	// endpoint and the signature are all derived from it.
	if err := t.authorizeHost(r.URL); err != nil {
		return nil, err
	}

	infinity := avanansig.IsInfinityHost(r.URL.Host)
	if infinity {
		r.URL.Path = avanansig.ApplyInfinityPrefix(r.URL.Path)
	}

	token, err := t.ensureToken(r, infinity)
	if err != nil {
		return nil, err
	}

	t.applyAuth(r, token, infinity)

	resp, err := t.base.RoundTrip(r)
	if err != nil {
		return nil, err
	}

	// A 401 on a request we authenticated means the cached token went stale
	// mid-session. Drop it and retry exactly once so a long-running sync does
	// not die on an hour boundary. Without the single-shot guard a genuinely
	// bad credential would loop.
	if resp.StatusCode == http.StatusUnauthorized && token != "" && t.credentialsConfigured() && !cliutil.IsVerifyEnv() {
		t.invalidate(token)
		fresh, mintErr := t.ensureToken(r, infinity)
		if mintErr != nil || fresh == "" || fresh == token {
			return resp, nil
		}
		// Build the retry BEFORE discarding the 401. If the body cannot be
		// replayed we must hand the caller the original response, and that is
		// only possible while it is still open — re-dispatching `r`, whose
		// body is already consumed, would ship a truncated request.
		retry := req.Clone(req.Context())
		if infinity {
			retry.URL.Path = avanansig.ApplyInfinityPrefix(retry.URL.Path)
		}
		if err := rewindBody(req, retry); err != nil {
			return resp, nil
		}
		drainAndClose(resp)
		t.applyAuth(retry, fresh, infinity)
		return t.base.RoundTrip(retry)
	}

	return resp, nil
}

// applyAuth stamps the auth headers onto a prepared request. The signature is
// computed last, over the request URL exactly as it will be sent.
func (t *AvananTransport) applyAuth(r *http.Request, token string, infinity bool) {
	reqID := uuid.NewString()
	r.Header.Set("x-av-req-id", reqID)

	if infinity {
		// Infinity Portal signs nothing; the bearer token is the credential.
		r.Header.Del("x-av-token")
		r.Header.Del("x-av-sig")
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		return
	}

	if t.cfg == nil {
		return
	}
	appID := t.cfg.AvananAppId
	if appID == "" {
		// Nothing to sign with. Clear the placeholder the generated client
		// installs from AuthHeader() so we do not send a bogus token value.
		r.Header.Del("x-av-token")
		return
	}

	date := avanansig.FormatDate(time.Now())
	// The handshake itself signs an empty request string; every other call
	// signs the path and query it is about to send.
	requestString := ""
	if !isHandshakePath(r.URL.Path) {
		requestString = avanansig.RequestString(r.URL)
	}

	r.Header.Set("x-av-app-id", appID)
	r.Header.Set("x-av-date", date)
	r.Header.Set("x-av-sig", avanansig.Sign(reqID, appID, date, requestString, t.cfg.ClientSecret))

	if token != "" {
		r.Header.Set("x-av-token", token)
	} else {
		// The generated client seeds x-av-token from AuthHeader(), which
		// resolves to the app id. Sending that as a token is worse than
		// sending nothing.
		r.Header.Del("x-av-token")
	}
}

func isHandshakePath(path string) bool {
	return strings.HasSuffix(strings.TrimSuffix(path, "/"), "/auth")
}

// ensureToken returns a usable session token, minting one if the cache is
// empty or expired. Returns an empty string (with no error) when there is
// nothing to mint from, so unauthenticated flows such as --dry-run and --help
// still work.
func (t *AvananTransport) ensureToken(r *http.Request, infinity bool) (string, error) {
	if isHandshakePath(r.URL.Path) {
		return "", nil
	}

	t.mu.Lock()
	if t.token != "" && time.Now().Before(t.expiry) {
		tok := t.token
		t.mu.Unlock()
		return tok, nil
	}
	t.mu.Unlock()

	if !t.credentialsConfigured() {
		// No credentials to mint with. Fall through unauthenticated; the
		// server's 401 is a clearer signal than a synthetic local error, and
		// dry-run paths never reach the wire at all.
		return "", nil
	}
	if cliutil.IsVerifyEnv() {
		// Mock-mode verification must not reach the real auth endpoint.
		return "", nil
	}

	tok, expiry, err := t.mint(r, infinity)
	if err != nil {
		return "", err
	}

	t.mu.Lock()
	t.token, t.expiry = tok, expiry
	t.mu.Unlock()
	return tok, nil
}

// invalidate clears the cached token, but only if it is still the one the
// caller saw. Guards against two concurrent 401s discarding a good token that
// a third goroutine already refreshed.
func (t *AvananTransport) invalidate(stale string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.token == stale {
		t.token = ""
		t.expiry = time.Time{}
	}
}

// mint performs the auth handshake appropriate to the host.
//
// The handshake endpoint is built from the configured base URL rather than from
// r.URL. RoundTrip already refuses off-host requests, so the two agree - but a
// credential-bearing endpoint should not be derived from a URL that request
// handling can redirect. Defence in depth for the leg that carries the secret.
func (t *AvananTransport) mint(r *http.Request, infinity bool) (string, time.Time, error) {
	cfgURL, err := url.Parse(strings.TrimSpace(t.cfg.BaseURL))
	if err != nil || cfgURL.Host == "" {
		return "", time.Time{}, fmt.Errorf("avanan: configured base URL %q is not a usable absolute URL", t.cfg.BaseURL)
	}
	baseURL := &url.URL{Scheme: cfgURL.Scheme, Host: cfgURL.Host}
	httpClient := &http.Client{Transport: t.base, Timeout: 30 * time.Second}
	if infinity {
		return t.mintInfinity(r, httpClient, baseURL)
	}
	return t.mintLegacy(r, httpClient, baseURL)
}

// mintLegacy performs the signed GET /v1.0/auth handshake. The response is the
// bare JWT as text on legacy hosts, but some deployments wrap it in the
// standard response envelope, so both shapes are accepted.
func (t *AvananTransport) mintLegacy(r *http.Request, httpClient *http.Client, baseURL *url.URL) (string, time.Time, error) {
	authURL := *baseURL
	authURL.Path = "/v1.0/auth"

	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, authURL.String(), nil)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("avanan auth handshake: %w", err)
	}
	reqID := uuid.NewString()
	date := avanansig.FormatDate(time.Now())
	req.Header.Set("Accept", "application/json")
	req.Header.Set("x-av-req-id", reqID)
	req.Header.Set("x-av-token", "")
	req.Header.Set("x-av-app-id", t.cfg.AvananAppId)
	req.Header.Set("x-av-date", date)
	req.Header.Set("x-av-sig", avanansig.Sign(reqID, t.cfg.AvananAppId, date, "", t.cfg.ClientSecret))

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("avanan auth handshake: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", time.Time{}, fmt.Errorf("avanan auth handshake: reading response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", time.Time{}, fmt.Errorf(
			"avanan auth handshake failed (HTTP %d): %s\ncheck AVANAN_APP_ID and AVANAN_CLIENT_SECRET, and confirm they were issued for this region (%s)",
			resp.StatusCode, t.scrub(truncate(string(body), 300)), baseURL.Host)
	}

	token := extractLegacyToken(body)
	if token == "" {
		return "", time.Time{}, fmt.Errorf("avanan auth handshake: no token in response from %s", authURL.String())
	}
	return token, time.Now().Add(legacyTokenTTL), nil
}

// extractLegacyToken pulls the JWT out of a handshake response. The documented
// shape is a bare text token; the envelope form is accepted defensively.
func extractLegacyToken(body []byte) string {
	raw := strings.TrimSpace(string(body))
	raw = strings.Trim(raw, `"`)
	if raw != "" && !strings.HasPrefix(raw, "{") {
		return raw
	}

	var envelope struct {
		ResponseData any `json:"responseData"`
		Data         struct {
			Token string `json:"token"`
		} `json:"data"`
		Token string `json:"token"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return ""
	}
	if envelope.Token != "" {
		return envelope.Token
	}
	if envelope.Data.Token != "" {
		return envelope.Data.Token
	}
	switch v := envelope.ResponseData.(type) {
	case string:
		return v
	case map[string]any:
		if s, ok := v["token"].(string); ok {
			return s
		}
	}
	return ""
}

// mintInfinity exchanges the access key for a bearer token on Infinity Portal
// hosts.
func (t *AvananTransport) mintInfinity(r *http.Request, httpClient *http.Client, baseURL *url.URL) (string, time.Time, error) {
	authURL := *baseURL
	authURL.Path = "/v2/auth/external"

	payload, err := json.Marshal(map[string]string{"accessKey": t.cfg.ClientSecret})
	if err != nil {
		return "", time.Time{}, fmt.Errorf("avanan infinity auth: %w", err)
	}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, authURL.String(), bytes.NewReader(payload))
	if err != nil {
		return "", time.Time{}, fmt.Errorf("avanan infinity auth: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("cloudinfra-external-client-id", t.cfg.AvananAppId)

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("avanan infinity auth: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", time.Time{}, fmt.Errorf("avanan infinity auth: reading response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", time.Time{}, fmt.Errorf(
			"avanan infinity auth failed (HTTP %d): %s\ncheck AVANAN_APP_ID (client id) and AVANAN_CLIENT_SECRET (access key)",
			resp.StatusCode, t.scrub(truncate(string(body), 300)))
	}

	var parsed struct {
		Data struct {
			Token     string      `json:"token"`
			ExpiresIn json.Number `json:"expiresIn"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", time.Time{}, fmt.Errorf("avanan infinity auth: parsing response: %w", err)
	}
	if parsed.Data.Token == "" {
		return "", time.Time{}, errors.New("avanan infinity auth: no token in response")
	}

	expiry := time.Now().Add(legacyTokenTTL)
	if secs, err := parsed.Data.ExpiresIn.Float64(); err == nil && secs > 0 {
		lifetime := time.Duration(secs) * time.Second
		if lifetime > tokenRefreshSkew {
			lifetime -= tokenRefreshSkew
		}
		expiry = time.Now().Add(lifetime)
	}
	return parsed.Data.Token, expiry, nil
}

// Mint forces a fresh handshake against baseURL and returns the token and its
// expiry. Used by `auth login` to persist a token without going through a data
// request first.
func (t *AvananTransport) Mint(req *http.Request) (string, time.Time, error) {
	if !t.credentialsConfigured() {
		return "", time.Time{}, errors.New("AVANAN_APP_ID and AVANAN_CLIENT_SECRET must both be set")
	}
	infinity := avanansig.IsInfinityHost(req.URL.Host)
	tok, expiry, err := t.mint(req, infinity)
	if err != nil {
		return "", time.Time{}, err
	}
	t.mu.Lock()
	t.token, t.expiry = tok, expiry
	t.mu.Unlock()
	return tok, expiry, nil
}

// AvananAuthTransport returns the signing transport installed on a client, if
// any. Lets hand-written commands drive the handshake directly.
func AvananAuthTransport(c *Client) *AvananTransport {
	if c == nil || c.HTTPClient == nil {
		return nil
	}
	t, _ := c.HTTPClient.Transport.(*AvananTransport)
	return t
}

// rewindBody re-attaches a replayable body to a retried request.
func rewindBody(orig, retry *http.Request) error {
	if orig.Body == nil || orig.Body == http.NoBody {
		return nil
	}
	if orig.GetBody == nil {
		return errors.New("request body is not replayable")
	}
	body, err := orig.GetBody()
	if err != nil {
		return err
	}
	retry.Body = body
	return nil
}

// drainAndClose releases a response we are discarding so the connection can be
// reused instead of being torn down.
func drainAndClose(resp *http.Response) {
	if resp == nil || resp.Body == nil {
		return
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	_ = resp.Body.Close()
}

// scrub removes credential material from text that came off the wire before it
// is interpolated into an error. The Infinity handshake POSTs
// {"accessKey": <secret>}, and a gateway that echoes the offending field on a
// 400 would otherwise print the secret to stderr.
func (t *AvananTransport) scrub(text string) string {
	text = strings.TrimSpace(text)
	if t == nil || t.cfg == nil {
		return text
	}
	for _, secret := range []string{t.cfg.ClientSecret, t.cfg.AvananToken, t.token} {
		if len(secret) >= 6 {
			text = strings.ReplaceAll(text, secret, "***redacted***")
		}
	}
	return text
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
