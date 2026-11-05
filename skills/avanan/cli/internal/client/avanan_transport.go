// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

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
	"avanan-pp-cli/internal/credkit"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
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
	// issued is every session token this transport has held, including ones
	// since discarded, so an error body echoing any of them is masked.
	issued []string
}

// OutboundGuard is the credential kit's transport. A variable so in-process
// tests, which run no credkit.Start and so have no declared origins, can stand
// up loopback servers; the built binaries are the proof of the guard itself.
var OutboundGuard = credkit.Transport

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
	// Hand-fix (handfixes.json credkit-wiring): the kit's guard sits inside the
	// signer, so it sees the signed headers, the handshakes and every redirect hop.
	t := &AvananTransport{base: OutboundGuard(base), cfg: c.Config}
	t.holdRuntime()
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

// holdRuntime holds with the credential kit every credential this transport has
// or obtains, the moment it has it and before any request carrying it is sent:
// the configured app id, client secret and stored session token (here on every
// call, so a token loaded from the config or credentials file is covered however
// the transport was built and however the config changed since), every session
// token either handshake mints (mint, and again on adoption), and the legacy
// handshake signature (mintLegacy).
func (t *AvananTransport) holdRuntime(obtained ...string) {
	cliutil.Hold("minted:session-token", obtained...)
	if t.cfg != nil {
		cliutil.Hold("config:unclassified", t.cfg.AvananAppId, t.cfg.ClientSecret, t.cfg.AvananToken)
	}
}

// adoptToken makes tok the cached session token. It is held first, and stays
// in issued after it is discarded so maskErrorBody keeps masking its echo.
func (t *AvananTransport) adoptToken(tok string, expiry time.Time) {
	t.holdRuntime(tok)
	t.mu.Lock()
	t.token, t.expiry = tok, expiry
	t.issued = append(t.issued, tok)
	t.mu.Unlock()
}

// holdsCredential reports whether this transport would attach any credential
// material to a request.
func (t *AvananTransport) holdsCredential() bool {
	if t.cfg != nil && (t.cfg.AvananAppId != "" || t.cfg.ClientSecret != "" || t.cfg.AvananToken != "") {
		return true
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.token != ""
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

// RoundTrip signs and dispatches a single request. Every non-2xx body it
// hands back has passed through maskErrorBody, and every body, any status,
// through withholdHeldBody. A request for an auth endpoint is refused: its
// answer is a session token, which only mint may receive (and hold), never a
// command, a redirect or a caller of the client as data.
func (t *AvananTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if isAuthEndpoint(req.URL.Path) {
		return nil, fmt.Errorf("avanan: refusing a request for the auth endpoint %s: session tokens are minted by the transport, never fetched as data", req.URL.Path)
	}
	resp, err := t.roundTrip(req)
	if err != nil {
		return nil, err
	}
	return t.maskErrorBody(resp)
}

// inflated decodes a Content-Encoding the server chose itself, as the client
// does before it prints a body, so the transport's checks read those bytes and
// not the compressed ones. The header is dropped once decoded.
func inflated(resp *http.Response, body []byte) []byte {
	if enc := resp.Header.Get("Content-Encoding"); enc != "" {
		if decoded, err := decodeContentEncoding(enc, body); err == nil {
			resp.Header.Del("Content-Encoding")
			return decoded
		}
	}
	return body
}

// maskErrorBody rewrites a non-2xx body so no credential this transport holds
// or has issued (app id, secret, stored token, every minted session token)
// reaches an error sink in any encoding the client's masker knows. The client
// masks only the credentials in Config; a minted token lives only here.
func (t *AvananTransport) maskErrorBody(resp *http.Response) (*http.Response, error) {
	if resp == nil || resp.Body == nil || (resp.StatusCode >= 200 && resp.StatusCode < 300) {
		return resp, nil
	}
	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		return nil, fmt.Errorf("avanan: reading response body: %w", err)
	}
	t.mu.Lock()
	creds := append([]string{t.token}, t.issued...)
	t.mu.Unlock()
	if t.cfg != nil {
		creds = append(creds, t.cfg.AvananAppId, t.cfg.ClientSecret, t.cfg.AvananToken)
	}
	masked := []byte((*Client)(nil).maskCredentialText(string(body), creds...))
	resp.Body = io.NopCloser(bytes.NewReader(masked))
	resp.ContentLength = int64(len(masked))
	resp.Header.Del("Content-Length")
	return resp, nil
}

func (t *AvananTransport) roundTrip(req *http.Request) (*http.Response, error) {
	// Before anything can send: the configured credentials are held.
	t.holdRuntime()

	// A RoundTripper must not mutate the request it is handed.
	r := req.Clone(req.Context())

	// Gate before anything reads the host: the scheme decision, the handshake
	// endpoint and the signature are all derived from it.
	if err := t.authorizeHost(r.URL); err != nil {
		return nil, err
	}
	// The transport is where credentials are attached (signature, app id,
	// stored or minted session token), so it enforces the plaintext rule for
	// everything it attaches. The generated client's own guard keys off
	// AuthHeader() (the app id) alone and misses a token-only config.
	if t.holdsCredential() {
		if err := plaintextCredentialError(r.URL, true); err != nil {
			return nil, err
		}
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
	// The envelope check runs before the 401 retry so an envelope-level 401
	// is retried exactly like an HTTP one.
	if resp, err = applyEnvelopeStatus(resp); err != nil {
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
		retried, err := t.base.RoundTrip(retry)
		if err != nil {
			return nil, err
		}
		return applyEnvelopeStatus(retried)
	}

	return resp, nil
}

// EnvelopeCodeFailed is the one rule for reading Avanan's vendor status.
//
// Every response is {responseEnvelope:{responseCode,...}, responseData}, and
// the vendor reports the real outcome in responseCode (spec components.schemas
// ResponseEnvelope); 0 and 200 are the success values the API uses. Anything
// else is a failure, whatever the HTTP status said.
func EnvelopeCodeFailed(code int) bool {
	return code != 0 && code != http.StatusOK
}

// envelopeResponseCode extracts responseEnvelope.responseCode. ok is false when
// the body carries no envelope code at all. A code that is present but not a
// number is reported as a failure (-1): any failure signal wins.
func envelopeResponseCode(body []byte) (code int, ok bool) {
	var probe struct {
		ResponseEnvelope *struct {
			ResponseCode json.RawMessage `json:"responseCode"`
		} `json:"responseEnvelope"`
	}
	if err := json.Unmarshal(body, &probe); err != nil || probe.ResponseEnvelope == nil {
		return 0, false
	}
	raw := strings.TrimSpace(string(probe.ResponseEnvelope.ResponseCode))
	if raw == "" || raw == "null" {
		return 0, false
	}
	raw = strings.Trim(raw, `"`)
	if f, err := strconv.ParseFloat(strings.TrimSpace(raw), 64); err == nil && f == float64(int(f)) {
		return int(f), true
	}
	return -1, true
}

// applyEnvelopeStatus turns an HTTP 2xx whose envelope reports failure into a
// non-2xx response, so every caller - generated commands, the MCP executor,
// hand-written commands - sees the vendor's verdict through the one status
// check it already makes. The body is preserved byte for byte. A vendor code
// that is a valid HTTP error status is used as-is; any other failing code
// becomes 502, since the upstream answered but did not succeed.
func applyEnvelopeStatus(resp *http.Response) (*http.Response, error) {
	if resp == nil || resp.Body == nil || resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return resp, nil
	}
	if ct := strings.ToLower(resp.Header.Get("Content-Type")); ct != "" && !strings.Contains(ct, "json") && !strings.HasPrefix(ct, "text/") {
		return resp, nil
	}
	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		return nil, fmt.Errorf("avanan: reading response body: %w", err)
	}
	body = inflated(resp, body)
	resp.Body = io.NopCloser(bytes.NewReader(body))
	resp.ContentLength = int64(len(body))
	resp.Header.Del("Content-Length")
	code, ok := envelopeResponseCode(body)
	if !ok || !EnvelopeCodeFailed(code) {
		return resp, nil
	}
	status := code
	if status < 400 || status > 599 {
		status = http.StatusBadGateway
	}
	resp.StatusCode = status
	resp.Status = fmt.Sprintf("%d %s", status, http.StatusText(status))
	resp.Header.Set("X-Avanan-Envelope-Response-Code", strconv.Itoa(code))
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
	// Held by the kit's transport, per request: credwire declares this header.
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

// isAuthEndpoint reports whether p names either handshake endpoint (legacy
// .../auth, Infinity .../auth/external) however it is spelled: any case, dot
// segments, doubled or trailing slashes.
func isAuthEndpoint(p string) bool {
	p = strings.ToLower(path.Clean("/" + p))
	return strings.HasSuffix(p, "/auth") || strings.HasSuffix(p, "/auth/external")
}

// holdAuthAnswer holds the token an auth answer carries the moment it is
// read, before any status, envelope or parse check can turn the answer into
// an error: a token the server issued is a credential whether or not this
// handshake goes on to use it.
func (t *AvananTransport) holdAuthAnswer(body []byte) {
	t.holdRuntime(extractLegacyToken(body))
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

	t.adoptToken(tok, expiry)
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
	// The handshake carries the credential itself (the Infinity access key in
	// the body, a replayable signature on legacy hosts) and runs on its own
	// client, outside the generated client's guards. So it enforces them
	// here: never over plaintext to a non-loopback host, and never through a
	// redirect - a 307 would replay the body to whatever host Location names.
	// No redirect appears anywhere in the vendor's handshake.
	if err := plaintextCredentialError(baseURL, true); err != nil {
		return "", time.Time{}, fmt.Errorf("avanan auth handshake: %w", err)
	}
	httpClient := &http.Client{
		Transport: t.base,
		Timeout:   30 * time.Second,
		CheckRedirect: func(req *http.Request, _ []*http.Request) error {
			return fmt.Errorf("refusing to follow a redirect to %s: the auth handshake carries the credential and is never redirected", req.URL.Host)
		},
	}
	mintOn := t.mintLegacy
	if infinity {
		mintOn = t.mintInfinity
	}
	tok, expiry, err := mintOn(r, httpClient, baseURL)
	if err != nil {
		return "", time.Time{}, err
	}
	// Held the moment it exists: before it is cached, returned to Mint's
	// caller, or attached to any request.
	t.holdRuntime(tok)
	return tok, expiry, nil
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
	// The handshake carries no token, so this signature is the credential on
	// this leg (replayable within the vendor's clock window): held before it
	// is sent.
	sig := avanansig.Sign(reqID, t.cfg.AvananAppId, date, "", t.cfg.ClientSecret)
	t.holdRuntime(sig)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("x-av-req-id", reqID)
	req.Header.Set("x-av-token", "")
	req.Header.Set("x-av-app-id", t.cfg.AvananAppId)
	req.Header.Set("x-av-date", date)
	req.Header.Set("x-av-sig", sig)

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("avanan auth handshake: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Status and a fixed diagnostic only. The body answers a request that
		// carried the credential, so it is never rendered: an echo in any
		// encoding would otherwise reach stderr or an MCP result.
		return "", time.Time{}, fmt.Errorf(
			"avanan auth handshake failed (HTTP %d)\ncheck AVANAN_APP_ID and AVANAN_CLIENT_SECRET, and confirm they were issued for this region (%s)",
			resp.StatusCode, baseURL.Host)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", time.Time{}, fmt.Errorf("avanan auth handshake: reading response: %w", err)
	}
	t.holdAuthAnswer(body)

	// An envelope-form answer that reports failure is not a token, whatever
	// responseData holds.
	if code, ok := envelopeResponseCode(body); ok && EnvelopeCodeFailed(code) {
		return "", time.Time{}, fmt.Errorf("avanan auth handshake failed (responseCode %d)\ncheck AVANAN_APP_ID and AVANAN_CLIENT_SECRET, and confirm they were issued for this region (%s)", code, baseURL.Host)
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

	if resp.StatusCode != http.StatusOK {
		// Status and a fixed diagnostic only; see mintLegacy.
		return "", time.Time{}, fmt.Errorf(
			"avanan infinity auth failed (HTTP %d)\ncheck AVANAN_APP_ID (client id) and AVANAN_CLIENT_SECRET (access key)",
			resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", time.Time{}, fmt.Errorf("avanan infinity auth: reading response: %w", err)
	}
	t.holdAuthAnswer(body)

	var parsed struct {
		Data struct {
			Token     string      `json:"token"`
			ExpiresIn json.Number `json:"expiresIn"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		// A fixed message: the decoder's error quotes the value it choked on,
		// which can be the token.
		return "", time.Time{}, errors.New("avanan infinity auth: the response is not the documented token shape")
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
	t.holdRuntime()
	if !t.credentialsConfigured() {
		return "", time.Time{}, errors.New("AVANAN_APP_ID and AVANAN_CLIENT_SECRET must both be set")
	}
	infinity := avanansig.IsInfinityHost(req.URL.Host)
	tok, expiry, err := t.mint(req, infinity)
	if err != nil {
		return "", time.Time{}, err
	}
	t.adoptToken(tok, expiry)
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
