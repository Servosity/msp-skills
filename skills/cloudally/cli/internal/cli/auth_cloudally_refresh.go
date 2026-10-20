// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"cloudally-pp-cli/internal/client"
	"cloudally-pp-cli/internal/cliutil"
)

// CloudAlly auth endpoints (relative to the configured base URL).
const (
	cloudallyPartnerLoginPath = "/v2/auth/partner"
	cloudallyAccountLoginPath = "/v2/auth"
	cloudallyRefreshPath      = "/v2/auth/refresh"
)

// cloudallyRefreshWindow is how close to expiry the access token may get
// before a new client refreshes it up front instead of letting the next
// request fail with a 401.
const cloudallyRefreshWindow = 60 * time.Second

// cloudallyRefreshTimeout bounds the refresh round trip. It is independent of
// the root --timeout so a generous download budget never stalls on auth.
const cloudallyRefreshTimeout = 15 * time.Second

// cloudallyDefaultAuthTimeout is used by the login commands when the root
// --timeout is unset or non-positive.
const cloudallyDefaultAuthTimeout = 30 * time.Second

// cloudallyMaxAuthBody caps how much of an auth response is read.
const cloudallyMaxAuthBody = 1 << 20

// Thresholds for the defensive expiresIn decoding. CloudAlly documents
// expiresIn as a duration but returns an absolute epoch in milliseconds in
// practice; some deployments return epoch seconds. Anything below the epoch
// seconds threshold is treated as a relative duration in seconds.
const (
	cloudallyEpochMillisThreshold  = int64(1e12)
	cloudallyEpochSecondsThreshold = int64(1e9)
)

// cloudallyRefreshRunning guards against re-entrant refreshes when a hook
// triggers while another refresh for the same process is in flight.
var cloudallyRefreshRunning atomic.Bool

func init() {
	clientHooks = append(clientHooks, cloudallyRefreshHook)
}

// cloudallyLoginHint is appended to every refresh failure so the user knows
// the recovery path without reading the code.
const cloudallyLoginHint = "run `cloudally-cli auth login` (CLOUDALLY_EMAIL / CLOUDALLY_PASSWORD), or `auth login --partner` for a partner session, to sign in again"

// cloudallyRefreshHook refreshes a stored access token that is expired or
// about to expire before the first request of a new client goes out. It is
// a no-op when there is nothing to refresh, during --dry-run, and under the
// mock verifier (which has no network) unless live HTTP is explicitly on.
func cloudallyRefreshHook(c *client.Client) error {
	if c == nil || c.Config == nil || c.DryRun {
		return nil
	}
	if cliutil.IsVerifyEnv() && !cliutil.IsVerifyLiveHTTPEnv() {
		return nil
	}
	cfg := c.Config
	if strings.TrimSpace(cfg.RefreshToken) == "" || cfg.TokenExpiry.IsZero() {
		return nil
	}
	if time.Until(cfg.TokenExpiry) > cloudallyRefreshWindow {
		return nil
	}
	if !cloudallyRefreshRunning.CompareAndSwap(false, true) {
		return nil
	}
	defer cloudallyRefreshRunning.Store(false)

	oldToken, oldRefresh := cfg.AccessToken, cfg.RefreshToken
	// A refresh token that does not belong to the active access token would
	// swap the credential to another session (and principal) in the middle
	// of a run that may already hold the old credential's local store open.
	// The pair is known to belong together only when the auth record vouches
	// for both tokens.
	actx, haveRecord := loadAuthContext()
	binding := bindingFor(cfg)
	pairVouched := haveRecord && actx.refreshPrincipal(binding, oldToken, oldRefresh) != ""
	if !pairVouched {
		if haveRecord && actx.activePrincipal(binding, oldToken) != "" {
			return authErr(fmt.Errorf("not refreshing the CloudAlly token: the stored refresh token does not belong to the signed-in session recorded in %s; %s", authContextFileName, cloudallyLoginHint))
		}
		// An access token from CLOUDALLY_ACCESS_TOKEN is used as given: the
		// stored refresh token (and the stored expiry this hook read)
		// belong to whatever session is stored, not to it.
		if env := strings.TrimSpace(cliutil.EnvOverride("CLOUDALLY_ACCESS_TOKEN")); env != "" && env == strings.TrimSpace(oldToken) {
			return nil
		}
	}
	accessToken, refreshToken, expiry, refreshExpiry, err := cloudallyRefreshTokens(c.BaseURL, cfg.ClientID, cfg.ClientSecret, cfg.RefreshToken)
	if err != nil {
		return err
	}
	if err := cfg.SaveTokens(cfg.ClientID, cfg.ClientSecret, accessToken, refreshToken, expiry); err != nil {
		return authErr(fmt.Errorf("saving refreshed CloudAlly token: %w; %s", err, cloudallyLoginHint))
	}
	// The auth record follows the rotation only when it described the
	// credential (access and refresh token, origin, client id) that was just
	// replaced. One describing anything else (a different config, an
	// environment token, another session's refresh token) is left exactly as
	// it is: neither its binding nor its kept partner session may take this
	// token.
	vouched := false
	if haveRecord {
		if principal := actx.refreshPrincipal(binding, oldToken, oldRefresh); principal != "" {
			vouched = true
			if principal == principalPartner {
				actx.stashPartner(binding, accessToken, refreshToken, expiry, true)
				if !refreshExpiry.IsZero() {
					actx.RefreshExpiresAt = refreshExpiry
				}
			}
			actx.bindTo(binding, accessToken, refreshToken, principal)
			// Sidecar persistence is best effort; the primary credential is
			// saved. A failure only means the next run cannot name the
			// local store by principal until the next login.
			if err := saveAuthContext(actx); err != nil {
				fmt.Fprintf(os.Stderr, "warning: could not update %s after the token refresh: %v\n", authContextFileName, err)
			}
		}
	}
	if !vouched {
		// The root command chose this run's local store for the credential
		// that was just replaced. Nothing vouches that the new token acts
		// as the same principal, so the rest of the run must not write into
		// that store: re-derive the scope from the new credential.
		setDefaultDBScopeIdentity(cfg.StoreScopeCredential(), cfg.StoreScopeLegacyCredential(), cfg.BaseURL)
	}
	return nil
}

// cloudallyRefreshTokens exchanges a refresh token for a new access token.
// When the response omits a refresh token the one passed in is returned so
// callers can keep storing it. Errors are authErr with the login hint.
func cloudallyRefreshTokens(baseURL, clientID, clientSecret, refreshToken string) (accessToken, newRefresh string, expiry, refreshExpiry time.Time, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), cloudallyRefreshTimeout)
	defer cancel()
	body, status, err := cloudallyAuthPost(ctx, baseURL, cloudallyRefreshPath, clientID, clientSecret, "",
		map[string]string{"refreshToken": refreshToken}, cloudallyRefreshTimeout)
	if err != nil {
		return "", "", time.Time{}, time.Time{}, authErr(fmt.Errorf("refreshing CloudAlly access token: %w; %s", err, cloudallyLoginHint))
	}
	if status < 200 || status >= 300 {
		return "", "", time.Time{}, time.Time{}, authErr(fmt.Errorf("refreshing CloudAlly access token: %w; %s", cloudallyAuthStatusError(status, body), cloudallyLoginHint))
	}
	accessToken, newRefresh, expiry, refreshExpiry, err = cloudallyParseTokenResponse(body)
	if err != nil {
		return "", "", time.Time{}, time.Time{}, authErr(fmt.Errorf("refreshing CloudAlly access token: %w; %s", err, cloudallyLoginHint))
	}
	if newRefresh == "" {
		// Some refresh responses omit the refresh token; keep the one we have.
		newRefresh = refreshToken
	}
	return accessToken, newRefresh, expiry, refreshExpiry, nil
}

// cloudallyDecodeExpiry turns the API's expiresIn value into an absolute time.
// Values above 1e12 are epoch milliseconds, values above 1e9 are epoch
// seconds, smaller positive values are seconds relative to now, and zero or
// negative values yield the zero time.
func cloudallyDecodeExpiry(expiresIn int64, now time.Time) time.Time {
	switch {
	case expiresIn <= 0:
		return time.Time{}
	case expiresIn > cloudallyEpochMillisThreshold:
		return time.UnixMilli(expiresIn).UTC()
	case expiresIn > cloudallyEpochSecondsThreshold:
		return time.Unix(expiresIn, 0).UTC()
	default:
		return now.Add(time.Duration(expiresIn) * time.Second).UTC()
	}
}

// cloudallyTokenResponse mirrors the login and refresh payloads. Numbers are
// decoded leniently because the API has been seen returning both integers
// and floats for the expiry fields.
type cloudallyTokenResponse struct {
	AccessToken           string      `json:"accessToken"`
	TokenType             string      `json:"tokenType"`
	ExpiresIn             json.Number `json:"expiresIn"`
	RefreshToken          string      `json:"refreshToken"`
	RefreshTokenExpiresIn json.Number `json:"refreshTokenExpiresIn"`
	ErrorMessage          string      `json:"errorMessage"`
}

// cloudallyParseTokenResponse decodes a login or refresh body. The refresh
// token and both expiries are optional; the access token is not.
func cloudallyParseTokenResponse(data []byte) (accessToken, refreshToken string, expiry, refreshExpiry time.Time, err error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return "", "", time.Time{}, time.Time{}, errors.New("empty token response")
	}
	var resp cloudallyTokenResponse
	dec := json.NewDecoder(bytes.NewReader(trimmed))
	dec.UseNumber()
	if decodeErr := dec.Decode(&resp); decodeErr != nil {
		return "", "", time.Time{}, time.Time{}, fmt.Errorf("decoding token response: %w", decodeErr)
	}
	if resp.AccessToken == "" {
		if resp.ErrorMessage != "" {
			return "", "", time.Time{}, time.Time{}, fmt.Errorf("token response: %s", resp.ErrorMessage)
		}
		return "", "", time.Time{}, time.Time{}, errors.New("token response has no accessToken")
	}
	now := time.Now()
	expiry = cloudallyDecodeExpiry(cloudallyNumberInt64(resp.ExpiresIn), now)
	refreshExpiry = cloudallyDecodeExpiry(cloudallyNumberInt64(resp.RefreshTokenExpiresIn), now)
	return resp.AccessToken, resp.RefreshToken, expiry, refreshExpiry, nil
}

// cloudallyNumberInt64 accepts integer or float JSON numbers; anything else
// decodes as zero, which the expiry decoder treats as "unknown".
func cloudallyNumberInt64(n json.Number) int64 {
	if n == "" {
		return 0
	}
	if i, err := n.Int64(); err == nil {
		return i
	}
	if f, err := n.Float64(); err == nil {
		return int64(f)
	}
	return 0
}

// cloudallyAuthPost sends a JSON POST with the CloudAlly client headers using
// a plain HTTP client. The auth endpoints run outside the generated client so
// the Authorization header stays under the caller's control: login sends
// none, use-account sends the partner bearer regardless of the active token.
func cloudallyAuthPost(ctx context.Context, baseURL, path, clientID, clientSecret, bearer string, body any, timeout time.Duration) ([]byte, int, error) {
	if timeout <= 0 {
		timeout = cloudallyDefaultAuthTimeout
	}
	var payload io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, 0, fmt.Errorf("encoding request body: %w", err)
		}
		payload = bytes.NewReader(encoded)
	}
	url := strings.TrimRight(baseURL, "/") + path
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, payload)
	if err != nil {
		return nil, 0, fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if clientID != "" {
		req.Header.Set("client-id", clientID)
	}
	if clientSecret != "" {
		req.Header.Set("client-secret", clientSecret)
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := cloudallyAuthHTTPClient(timeout).Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("POST %s: %w", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		return nil, resp.StatusCode, cloudallyAuthRedirectError(path, resp)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, cloudallyMaxAuthBody))
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("reading response from %s: %w", path, err)
	}
	return data, resp.StatusCode, nil
}

// cloudallyAuthHTTPClient is the client every auth POST goes through. It
// never follows a redirect: a login, refresh, or token mint carries the client
// secret and either a password, a refresh token, or the partner bearer, and
// Go's default policy re-sends custom headers and (on a 307 or 308) the body
// to whatever host the Location names. No CloudAlly auth endpoint redirects,
// so a 3xx is answered as an error and nothing it returned is saved.
func cloudallyAuthHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

// cloudallyAuthRedirectError explains a refused redirect. It names only the
// target's scheme and host: a Location can carry a token in its query.
func cloudallyAuthRedirectError(path string, resp *http.Response) error {
	target := "an unnamed location"
	if loc, err := resp.Location(); err == nil && loc.Host != "" {
		target = loc.Scheme + "://" + loc.Host
	}
	return fmt.Errorf("POST %s answered HTTP %d redirecting to %s; auth requests never follow a redirect, so the credentials were not re-sent and no token was saved (check CLOUDALLY_BASE_URL)",
		path, resp.StatusCode, target)
}

// cloudallyAuthStatusError turns a non-2xx auth response into a message that
// names the credential to fix. 401 carries {"errorMessage": ...}; 403 comes
// back with an empty body when the client headers are missing or wrong.
func cloudallyAuthStatusError(status int, body []byte) error {
	var parsed struct {
		ErrorMessage string `json:"errorMessage"`
	}
	msg := ""
	if json.Unmarshal(body, &parsed) == nil {
		msg = strings.TrimSpace(parsed.ErrorMessage)
	}
	switch status {
	case http.StatusUnauthorized:
		if msg == "" {
			msg = "invalid credentials"
		}
		return fmt.Errorf("HTTP 401: %s (check --email/--password or CLOUDALLY_PARTNER_EMAIL/CLOUDALLY_PARTNER_PASSWORD)", msg)
	case http.StatusForbidden:
		if msg == "" {
			msg = "forbidden"
		}
		return fmt.Errorf("HTTP 403: %s (check CLOUDALLY_CLIENT_ID and CLOUDALLY_CLIENT_SECRET, and that the token is still valid)", msg)
	default:
		if msg == "" {
			msg = strings.TrimSpace(string(body))
			if len(msg) > 200 {
				msg = msg[:200]
			}
		}
		if msg == "" {
			msg = http.StatusText(status)
		}
		return fmt.Errorf("HTTP %d: %s", status, msg)
	}
}
