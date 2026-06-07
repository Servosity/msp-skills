// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-wired client-credentials token minting for the Action1 API.
//
// Action1's OAuth2 token endpoint is a quirk: the spec declares a "password"
// flow but the real grant is client-credentials, and the request body is
// application/json (NOT the usual form-encoded client_credentials). This file
// implements automatic minting + refresh from ACTION1_CLIENT_ID /
// ACTION1_CLIENT_SECRET so generated commands authenticate headlessly without
// the user pre-pasting a bearer token.

package client

import (
	"action1-pp-cli/internal/cliutil"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
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

// mintClientCredentials obtains (or refreshes) a bearer token from the
// configured client_id/client_secret and caches it on the config. It returns
// "Bearer <token>" or an error. It never dials during --dry-run or the verify
// short-circuit, so previews and mock-mode verification stay offline.
func (c *Client) mintClientCredentials(ctx context.Context) (string, error) {
	if c.Config == nil || c.Config.ClientID == "" || c.Config.ClientSecret == "" {
		return "", nil
	}
	// Never mint during preview/verify: --dry-run shows the request without a
	// live token, and the verify short-circuit must not dial out or write a
	// token to the user's real config.
	if c.DryRun || (cliutil.IsVerifyEnv() && !cliutil.IsVerifyLiveHTTPEnv()) {
		return "", nil
	}

	refreshing := c.Config.RefreshToken != "" && c.Config.AccessToken != ""
	var body map[string]any
	if refreshing {
		// Prefer refresh when we already had a token (cheaper, no re-auth).
		body = map[string]any{
			"client_id":     c.Config.ClientID,
			"refresh_token": c.Config.RefreshToken,
			"grant_type":    "refresh_token",
		}
	} else {
		body = map[string]any{
			"client_id":     c.Config.ClientID,
			"client_secret": c.Config.ClientSecret,
		}
	}
	raw, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, "POST", c.BaseURL+"/oauth2/token", strings.NewReader(string(raw)))
	if err != nil {
		return "", fmt.Errorf("building token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("requesting Action1 token: %w", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode/100 != 2 {
		// On a failed refresh, drop the stale refresh token and fall back to a
		// full client-credentials grant exactly once.
		if refreshing {
			c.Config.RefreshToken = ""
			c.Config.AccessToken = ""
			return c.mintClientCredentials(ctx)
		}
		// Never echo the raw response body: a misconfigured or echoing endpoint
		// behind a user-controlled BaseURL could reflect the posted client_secret,
		// and substring masking can be evaded by encoding. Only a strictly
		// validated RFC-6749 error code survives into the user-visible error.
		return "", fmt.Errorf("Action1 token endpoint returned HTTP %d%s — check ACTION1_CLIENT_ID / ACTION1_CLIENT_SECRET", resp.StatusCode, tokenErrCode(data))
	}
	var tr tokenResponse
	if err := json.Unmarshal(data, &tr); err != nil {
		return "", fmt.Errorf("parsing token response: %w", err)
	}
	if tr.AccessToken == "" {
		return "", fmt.Errorf("Action1 token endpoint returned no access_token")
	}
	var expiry time.Time
	if tr.ExpiresIn > 0 {
		expiry = time.Now().Add(time.Duration(tr.ExpiresIn) * time.Second)
	}
	// Best-effort cache to disk so subsequent invocations reuse the token until
	// it expires. A save failure is non-fatal: the token still works this run.
	_ = c.Config.SaveTokens(c.Config.ClientID, c.Config.ClientSecret, tr.AccessToken, tr.RefreshToken, expiry)
	return "Bearer " + tr.AccessToken, nil
}
