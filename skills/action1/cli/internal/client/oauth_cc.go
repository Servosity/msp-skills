// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-wired client-credentials token minting for the Action1 API
// (action1 handfix: client-credentials-mint).
//
// Action1's OAuth2 token endpoint is a quirk: the spec declares a "password"
// flow but the real grant is client credentials, and the request body is
// application/json (NOT the usual form-encoded client_credentials). The token
// endpoint lives on the API origin (<base URL>/oauth2/token), so a region
// base URL (EU, AU) mints from its own region and the credential never goes
// to another host. Generated commands authenticate headlessly from
// ACTION1_CLIENT_ID / ACTION1_CLIENT_SECRET without a pasted bearer token.

package client

import (
	"action1-pp-cli/internal/cliutil"
	"action1-pp-cli/internal/config"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// tokenResponse models POST /oauth2/token. Action1 returns the bearer token, a
// refresh token, the lifetime in seconds, and the token type.
type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
}

// mintMu serialises minting so concurrent requests (sync fans out across
// organizations) mint one token instead of racing on the config fields.
// ponytail: one process-wide lock; a per-client lock if two configs ever mint at once.
var mintMu sync.Mutex

// pairTokens holds the token this process minted for each client pair (keyed
// by a hash of the base URL, client ID and client secret), so every client
// the process builds for one pair reuses one token, and a token is only ever
// sent for the pair that minted it (action1 handfix
// minted-token-bound-to-its-client; Codex r1 action1 class cached
// authentication ignores credential override: the stored token of the files'
// client authenticated an environment pair that names another client, so a
// command returned the other tenant's data).
var pairTokens = struct {
	sync.Mutex
	m map[string]pairToken
}{m: map[string]pairToken{}}

type pairToken struct {
	access, refresh string
	expiry          time.Time
}

func pairKey(base string, cfg *config.Config) string {
	sum := sha256.Sum256([]byte(base + "\x00" + cfg.ClientID + "\x00" + cfg.ClientSecret))
	return hex.EncodeToString(sum[:])
}

// mintedHeader is the header of this process's unexpired token for the
// client's pair, "" when there is none.
func (c *Client) mintedHeader() string {
	pairTokens.Lock()
	defer pairTokens.Unlock()
	t, ok := pairTokens.m[pairKey(c.BaseURL, c.Config)]
	if !ok || t.access == "" || (!t.expiry.IsZero() && time.Now().After(t.expiry.Add(-60*time.Second))) {
		return ""
	}
	return "Bearer " + t.access
}

// tokenErrCode extracts the RFC-6749 "error" code from a failed token-mint
// response for diagnostics. Only a short, charset-restricted code is returned;
// anything else (including error_description, which could echo request data)
// is dropped.
func tokenErrCode(data []byte) string {
	var e struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(data, &e) != nil || e.Error == "" || len(e.Error) > 64 {
		return ""
	}
	for _, r := range e.Error {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '-':
		default:
			return ""
		}
	}
	return " (" + e.Error + ")"
}

// clientCredentialsHeader returns the Authorization header for a request. An
// explicit token (a configured header, ACTION1_OAUTH2) is used as is. With a
// client pair configured, the header is a token minted for THAT pair: this
// process's own, or the stored token when the files recorded this same client
// with it (config.StoredTokenOwned), else a new mint. A stored token of
// another client, or of none, is never sent for the pair.
func (c *Client) clientCredentialsHeader(ctx context.Context, authHeader string) (string, error) {
	cfg := c.Config
	if cfg == nil || cfg.ClientID == "" || cfg.ClientSecret == "" {
		return authHeader, nil
	}
	if cfg.AuthHeaderVal != "" || cfg.Action1Oauth2 != "" {
		return authHeader, nil
	}
	if h := c.mintedHeader(); h != "" {
		return h, nil
	}
	owned := cfg.StoredTokenOwned()
	if authHeader != "" && owned && !cfg.AccessTokenExpired() {
		return authHeader, nil
	}
	h, err := c.mintClientCredentials(ctx)
	if err != nil {
		if authHeader != "" && owned {
			// This client's expired token and a failed re-mint: send the old
			// header so the request fails with the API's own 401, not a
			// silent success.
			return authHeader, nil
		}
		return "", err
	}
	if h == "" && !owned {
		return "", nil // a preview: nothing minted, and the stored token is not this client's
	}
	if h == "" {
		return authHeader, nil
	}
	return h, nil
}

// mintClientCredentials obtains (or refreshes) a bearer token from the
// configured client_id/client_secret and caches it on the config. It returns
// "Bearer <token>" or an error. It never dials during --dry-run or the verify
// short-circuit, so previews and mock-mode verification stay offline.
func (c *Client) mintClientCredentials(ctx context.Context) (string, error) {
	mintMu.Lock()
	defer mintMu.Unlock()
	cfg := c.Config
	if cfg == nil || cfg.ClientID == "" || cfg.ClientSecret == "" {
		return "", nil
	}
	if c.DryRun || (cliutil.IsVerifyEnv() && !cliutil.IsVerifyLiveHTTPEnv()) {
		return "", nil
	}
	// Another request minted while this one waited for the lock.
	if h := c.mintedHeader(); h != "" {
		return h, nil
	}
	owned := cfg.StoredTokenOwned()
	if owned && !cfg.AccessTokenExpired() {
		return "Bearer " + cfg.AccessToken, nil
	}
	if owned && cfg.RefreshToken != "" {
		// Prefer refresh when a token existed (cheaper, no re-auth). A failed
		// refresh drops the stale pair and falls back to one full grant.
		if h, err := c.postTokenRequest(ctx, map[string]any{
			"client_id":     cfg.ClientID,
			"refresh_token": cfg.RefreshToken,
			"grant_type":    "refresh_token",
		}); err == nil {
			return h, nil
		}
		cfg.RefreshToken = ""
		cfg.AccessToken = ""
	}
	return c.postTokenRequest(ctx, map[string]any{
		"client_id":     cfg.ClientID,
		"client_secret": cfg.ClientSecret,
	})
}

func (c *Client) postTokenRequest(ctx context.Context, body map[string]any) (string, error) {
	cfg := c.Config
	raw, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.BaseURL, "/")+"/oauth2/token", strings.NewReader(string(raw)))
	if err != nil {
		return "", fmt.Errorf("building token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := (&http.Client{Transport: c.HTTPClient.Transport, Timeout: c.HTTPClient.Timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}).Do(req) // a token request never follows a redirect (Codex r1 P2 token-redirect-deadlock: the redirect hook re-derived auth under mintMu)
	if err != nil {
		// The transport error names the token URL (no query, no body); the
		// output chokepoint withholds any held value that still appears.
		return "", fmt.Errorf("requesting Action1 token: %w", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode/100 != 2 {
		// Never echo the raw response body: an echoing endpoint could reflect the
		// posted client_secret, and substring masking can be evaded by encoding.
		// Only a strictly validated RFC-6749 error code survives into the error.
		return "", fmt.Errorf("Action1 token endpoint returned HTTP %d%s; check ACTION1_CLIENT_ID / ACTION1_CLIENT_SECRET", resp.StatusCode, tokenErrCode(data))
	}
	var tr tokenResponse
	if err := json.Unmarshal(data, &tr); err != nil {
		return "", fmt.Errorf("parsing token response: not JSON")
	}
	if tr.AccessToken == "" {
		return "", fmt.Errorf("Action1 token endpoint returned no access_token")
	}
	// Hand-fix (handfixes.json credkit-wiring): a minted token is held before anything can print it.
	cliutil.Hold("minted:access_token", tr.AccessToken)
	cliutil.Hold("minted:refresh_token", tr.RefreshToken)
	var expiry time.Time
	if tr.ExpiresIn > 0 {
		expiry = time.Now().Add(time.Duration(tr.ExpiresIn) * time.Second)
	}
	pairTokens.Lock()
	pairTokens.m[pairKey(c.BaseURL, cfg)] = pairToken{access: tr.AccessToken, refresh: tr.RefreshToken, expiry: expiry}
	pairTokens.Unlock()
	// Best-effort cache in the 0600 credential store so later invocations reuse
	// the token until it expires (a no-op under ACTION1_NO_CONFIG_WRITE), only
	// when the pair in effect is the one the files hold: the token is stored
	// beside the client it belongs to. A pair the environment supplies keeps
	// its token in this process (it is never written, and a stored token would
	// read as the files' client's next run). A save failure is non-fatal: the
	// token still works this run.
	if cfg.PairIsStored() {
		_ = cfg.SaveTokens(cfg.ClientID, cfg.ClientSecret, tr.AccessToken, tr.RefreshToken, expiry)
	}
	return "Bearer " + tr.AccessToken, nil
}
