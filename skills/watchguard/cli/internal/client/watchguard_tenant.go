// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"watchguard-pp-cli/internal/cliutil"
)

// WatchGuard Service Provider credentials reach a managed (Subscriber) account's
// resources only with an access token minted with that account's `audience`
// (POST /oauth/token ... &audience=<value>); the value comes from
// POST /rest/platform/authorization/v1/audiences. Source:
// https://www.watchguard.com/help/docs/API/Content/en-US/api_get_started/authentication.html
//
// Audience tokens live in memory only. The persisted credential stays the
// caller's own token, so switching accounts never reuses a wrong-scope token.

const (
	// The OAuth endpoint URL is public; the name avoids gosec G101's credential-name pattern.
	defaultOAuthEndpoint = "https://api.usa.cloud.watchguard.com/oauth/token"
	audiencesPath        = "/rest/platform/authorization/v1/audiences"
	// AudienceEnvVar scopes every request a process makes to one managed account.
	AudienceEnvVar = "WATCHGUARD_AUDIENCE"
)

type scopedToken struct {
	bearer string
	expiry time.Time
}

// ponytail: per-process caches; a disk cache keyed by audience would save one
// token request per CLI invocation if the 200k/day quota ever becomes tight.
var (
	scopedMu      sync.Mutex
	scopedTokens  = map[string]scopedToken{} // clientID|tokenURL|audience -> token
	scopedBearers = map[string]bool{}        // every bearer we minted, for the transport
	audienceCache = map[string]string{}      // clientID|baseURL|accountID -> audience
)

func (c *Client) tokenEndpoint() string {
	if c.Config != nil && strings.TrimSpace(c.Config.TokenURL) != "" {
		return strings.TrimSpace(c.Config.TokenURL)
	}
	return defaultOAuthEndpoint
}

func (c *Client) scopedKey(audience string) string {
	id, _ := resolveClientCredentials(c.Config)
	return id + "|" + c.tokenEndpoint() + "|" + audience
}

// ScopedBearer returns an "Authorization" value minted for audience, reusing a
// cached token until two minutes before it expires.
func (c *Client) ScopedBearer(ctx context.Context, audience string) (string, time.Time, error) {
	audience = strings.TrimSpace(audience)
	if audience == "" {
		return "", time.Time{}, fmt.Errorf("audience is empty")
	}
	// hand-fix (handfixes.json dry-run-never-mints): no audience token request
	// from a --dry-run process.
	if c.DryRun {
		return "", time.Time{}, ErrDryRunNoTokenRequest
	}
	key := c.scopedKey(audience)
	scopedMu.Lock()
	tok, ok := scopedTokens[key]
	scopedMu.Unlock()
	if ok && time.Until(tok.expiry) > 2*time.Minute {
		return tok.bearer, tok.expiry, nil
	}
	clientID, clientSecret := resolveClientCredentials(c.Config)
	if clientID == "" || clientSecret == "" {
		return "", time.Time{}, fmt.Errorf("minting a managed-account token needs WATCHGUARD_CLIENT_ID (Access ID) and WATCHGUARD_CLIENT_SECRET (API password)")
	}
	form := url.Values{"grant_type": {"client_credentials"}, "audience": {audience}}
	if scope := resolveClientCredentialsScope(); scope != "" {
		form.Set("scope", scope)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.tokenEndpoint(), strings.NewReader(form.Encode()))
	if err != nil {
		return "", time.Time{}, fmt.Errorf("building token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", resolveClientCredentialsUserAgent())
	req.SetBasicAuth(clientID, clientSecret)
	apiKey := ""
	if c.Config != nil {
		apiKey = c.Config.WatchguardApiKey
	}
	resp, err := cliutil.OAuthTokenHTTPClient(c.HTTPClient).Do(req)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("calling token endpoint: %w", SafeTokenEndpointError(err, c.tokenEndpoint(), clientID, clientSecret, apiKey))
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 400 {
		return "", time.Time{}, &APIError{Method: http.MethodPost, Path: "/oauth/token (audience)", StatusCode: resp.StatusCode, Body: SafeTokenEndpointBody(body, clientID, clientSecret, apiKey)}
	}
	var tr struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &tr); err != nil || tr.AccessToken == "" {
		return "", time.Time{}, fmt.Errorf("token endpoint returned no access_token for the audience")
	}
	ttl := time.Duration(tr.ExpiresIn) * time.Second
	if ttl <= 0 {
		ttl = 55 * time.Minute // documented lifetime is 3600 s
	}
	tok = scopedToken{bearer: "Bearer " + tr.AccessToken, expiry: time.Now().Add(ttl)}
	scopedMu.Lock()
	scopedTokens[key] = tok
	scopedBearers[tok.bearer] = true
	scopedMu.Unlock()
	return tok.bearer, tok.expiry, nil
}

func (c *Client) dropScoped(audience string) {
	scopedMu.Lock()
	delete(scopedTokens, c.scopedKey(audience))
	scopedMu.Unlock()
}

func isScopedBearer(v string) bool {
	scopedMu.Lock()
	defer scopedMu.Unlock()
	return scopedBearers[v]
}

// AudienceFor returns the audience value for a managed account, using the
// caller's own token.
func (c *Client) AudienceFor(ctx context.Context, accountID string) (string, error) {
	accountID = strings.TrimSpace(accountID)
	if c.DryRun {
		return "", fmt.Errorf("looking up audience for %s: %w", accountID, ErrDryRunNoTokenRequest)
	}
	id, _ := resolveClientCredentials(c.Config)
	key := id + "|" + c.BaseURL + "|" + accountID
	scopedMu.Lock()
	aud, ok := audienceCache[key]
	scopedMu.Unlock()
	if ok {
		return aud, nil
	}
	data, _, err := c.PostQueryWithParams(ctx, audiencesPath, nil, map[string]string{"accountId": accountID})
	if err != nil {
		return "", fmt.Errorf("looking up audience for %s: %w", accountID, err)
	}
	var out struct {
		Audience string `json:"audience"`
	}
	if err := json.Unmarshal(data, &out); err != nil || strings.TrimSpace(out.Audience) == "" {
		return "", fmt.Errorf("looking up audience for %s: response had no audience", accountID)
	}
	scopedMu.Lock()
	audienceCache[key] = out.Audience
	scopedMu.Unlock()
	return out.Audience, nil
}

// TenantHeaders returns per-request headers that scope a call to accountID.
// The caller's own account (the WATCHGUARD_ACCOUNT_ID / --account-id value)
// needs no override and gets nil.
func (c *Client) TenantHeaders(ctx context.Context, accountID string) (map[string]string, error) {
	if accountID == "" || accountID == c.OwnAccountID() {
		return nil, nil
	}
	aud, err := c.AudienceFor(ctx, accountID)
	if err != nil {
		return nil, err
	}
	bearer, _, err := c.ScopedBearer(ctx, aud)
	if err != nil {
		return nil, fmt.Errorf("minting token for %s: %w", accountID, err)
	}
	return map[string]string{"Authorization": bearer, scopedAudienceHeader: aud}, nil
}

// WarmAuth makes sure the caller's own token is minted before goroutines share
// the client (the generated mint is not safe to start concurrently).
func (c *Client) WarmAuth(ctx context.Context) error {
	if c.DryRun || (cliutil.IsVerifyEnv() && !cliutil.IsVerifyLiveHTTPEnv()) {
		return nil
	}
	_, err := c.authHeader(ctx)
	return err
}

// OwnAccountID is the account the credentials belong to, as configured.
func (c *Client) OwnAccountID() string {
	if c.Config == nil || c.Config.TemplateVars == nil {
		return ""
	}
	return strings.TrimSpace(c.Config.TemplateVars["accountId"])
}

// ScrubScoped removes any audience bearer we minted from an error's text, so a
// server that echoes request headers cannot leak one into JSON output.
func ScrubScoped(err error) error {
	if err == nil {
		return nil
	}
	msg := scrubText(err.Error())
	if msg == err.Error() {
		return err
	}
	return &scrubbedError{msg: msg, err: err}
}

type scrubbedError struct {
	msg string
	err error
}

func (e *scrubbedError) Error() string { return e.msg }
func (e *scrubbedError) Unwrap() error { return e.err }

// scopedAudienceHeader tells scopeTransport which audience a request's bearer
// belongs to; it is stripped before the request leaves the process.
const scopedAudienceHeader = "X-Pp-Watchguard-Audience"

// scopeTransport keeps audience-scoped requests scoped:
//   - With WATCHGUARD_AUDIENCE set, every API request (not the token endpoint)
//     carries that audience's bearer, so the generated client's own-token
//     re-mint near expiry or after a 401 can never widen the scope.
//   - A request whose scoped bearer is rejected (401) is retried once with a
//     fresh scoped token; a second 401 is returned as 403 so the generated
//     handler does not re-mint the caller's own token, which cannot help.
//   - Scoped bearers go only to the configured API origin (a redirect to any
//     other host is sent without one), and error bodies are scrubbed of them.
type scopeTransport struct {
	base     http.RoundTripper
	c        *Client
	origin   string // scheme://host of the API base URL
	audience string // WATCHGUARD_AUDIENCE, or "" when only fan-out requests are scoped
}

func originOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return ""
	}
	return strings.ToLower(u.Scheme + "://" + u.Host)
}

func (t *scopeTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Off the API origin (a redirect, a stray absolute URL) nothing scoped is sent.
	if t.origin == "" || originOf(req.URL.String()) != t.origin {
		if req.Header.Get(scopedAudienceHeader) == "" && !isScopedBearer(req.Header.Get("Authorization")) {
			return t.base.RoundTrip(req)
		}
		r := req.Clone(req.Context())
		r.Header.Del(scopedAudienceHeader)
		if isScopedBearer(r.Header.Get("Authorization")) {
			r.Header.Del("Authorization")
		}
		return t.base.RoundTrip(r)
	}
	// Token exchanges carry Basic credentials and are never re-scoped.
	if strings.HasPrefix(req.Header.Get("Authorization"), "Basic ") {
		return t.base.RoundTrip(req)
	}
	aud := req.Header.Get(scopedAudienceHeader)
	if aud == "" && !isScopedBearer(req.Header.Get("Authorization")) {
		aud = t.audience
	}
	if aud == "" {
		return t.base.RoundTrip(req)
	}
	send := func(fresh bool) (*http.Response, error) {
		if fresh {
			t.c.dropScoped(aud)
		}
		bearer, _, err := t.c.ScopedBearer(req.Context(), aud)
		if err != nil {
			return nil, err
		}
		r := req.Clone(req.Context())
		r.Header.Del(scopedAudienceHeader)
		r.Header.Set("Authorization", bearer)
		if req.GetBody != nil {
			if r.Body, err = req.GetBody(); err != nil {
				return nil, err
			}
		}
		return t.base.RoundTrip(r)
	}
	resp, err := send(false)
	if err == nil && resp.StatusCode == http.StatusUnauthorized {
		_ = resp.Body.Close()
		if resp, err = send(true); err == nil && resp.StatusCode == http.StatusUnauthorized {
			resp.StatusCode, resp.Status = http.StatusForbidden, "403 Forbidden"
			resp.Body = prefixBody(resp.Body, "managed-account token rejected (HTTP 401): ")
		}
	}
	if err != nil || resp.StatusCode < 400 {
		return resp, err
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	_ = resp.Body.Close()
	resp.Body = io.NopCloser(strings.NewReader(scrubText(string(body))))
	resp.ContentLength = -1
	resp.Header.Del("Content-Length")
	return resp, nil
}

func prefixBody(b io.ReadCloser, prefix string) io.ReadCloser {
	body, _ := io.ReadAll(io.LimitReader(b, 1<<20))
	_ = b.Close()
	return io.NopCloser(bytes.NewReader(append([]byte(prefix), body...)))
}

// scrubText replaces every scoped bearer this process minted with ****.
func scrubText(s string) string {
	scopedMu.Lock()
	defer scopedMu.Unlock()
	for b := range scopedBearers {
		s = strings.ReplaceAll(s, strings.TrimPrefix(b, "Bearer "), "****")
	}
	return s
}

// InstallScopeTransport wraps the client's transport (see scopeTransport). With
// WATCHGUARD_AUDIENCE set it also mints that audience's token up front so a
// bad audience fails before any command runs.
func InstallScopeTransport(c *Client) error {
	if c == nil || c.HTTPClient == nil {
		return nil
	}
	// EnvOverride treats an unresolved .mcpb ${user_config...} placeholder as unset.
	audience := strings.TrimSpace(cliutil.EnvOverride(AudienceEnvVar))
	base := c.HTTPClient.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	if _, wrapped := base.(*scopeTransport); !wrapped {
		c.HTTPClient.Transport = &scopeTransport{base: base, c: c, origin: originOf(c.RequestBaseURL()), audience: audience}
	}
	if audience == "" {
		return nil
	}
	// The response cache is keyed on the configured (own) credential, not the
	// audience, so a scoped process must neither read nor seed it.
	c.NoCache = true
	if c.DryRun || (cliutil.IsVerifyEnv() && !cliutil.IsVerifyLiveHTTPEnv()) {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, _, err := c.ScopedBearer(ctx, audience); err != nil {
		return fmt.Errorf("%s is set but its token could not be minted: %w", AudienceEnvVar, err)
	}
	return nil
}
