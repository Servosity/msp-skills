// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
//
// Datto RMM composed-OAuth2 token mint. Datto RMM does not issue long-lived
// API keys directly; instead the user has an API key + API secret key which
// are exchanged for a bearer token via an OAuth2 password grant against a
// fixed public client. This file is hand-authored (not generator output) and
// implements that exchange plus a cache-aware ensure helper.
package config

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// dattoPlatformURLs maps each Datto RMM regional platform to its API host.
// An MSP account lives on exactly one platform; the user selects it with
// DATTO_RMM_PLATFORM (or sets DATTO_RMM_API_URL / DATTO_RMM_BASE_URL directly).
var dattoPlatformURLs = map[string]string{
	"pinotage":  "https://pinotage-api.centrastage.net",
	"merlot":    "https://merlot-api.centrastage.net",
	"concord":   "https://concord-api.centrastage.net",
	"vidal":     "https://vidal-api.centrastage.net",
	"zinfandel": "https://zinfandel-api.centrastage.net",
	"syrah":     "https://syrah-api.centrastage.net",
}

// ResolveDattoBaseURL sets cfg.BaseURL from DATTO_RMM_PLATFORM when no explicit
// base URL was provided. Precedence: DATTO_RMM_API_URL / DATTO_RMM_BASE_URL env
// (already applied by Load) > DATTO_RMM_PLATFORM > config file / default.
func ResolveDattoBaseURL(cfg *Config) error {
	if cfg == nil {
		return nil
	}
	// An explicit base URL env var already won in Load(); respect it.
	if os.Getenv("DATTO_RMM_BASE_URL") != "" {
		return nil
	}
	// DATTO_RMM_API_URL is an accepted alias for the full base. Accept either
	// the host root or a value already ending in /api.
	if v := strings.TrimSpace(os.Getenv("DATTO_RMM_API_URL")); v != "" {
		b := strings.TrimRight(v, "/")
		if !strings.HasSuffix(b, "/api") {
			b += "/api"
		}
		cfg.BaseURL = b
		return nil
	}
	plat := strings.ToLower(strings.TrimSpace(os.Getenv("DATTO_RMM_PLATFORM")))
	if plat == "" {
		return nil
	}
	base, ok := dattoPlatformURLs[plat]
	if !ok {
		return fmt.Errorf("unknown DATTO_RMM_PLATFORM %q (valid: pinotage, merlot, concord, vidal, zinfandel, syrah)", plat)
	}
	cfg.BaseURL = base + "/api"
	return nil
}

// dattoTokenEndpoint derives the OAuth token endpoint from the API base URL.
// BaseURL is like https://merlot-api.centrastage.net/api ; the token endpoint
// lives at the host root: https://<platform>-api.centrastage.net/auth/oauth/token.
func dattoTokenEndpoint(baseURL string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("invalid base URL %q (set DATTO_RMM_PLATFORM or DATTO_RMM_API_URL)", baseURL)
	}
	return u.Scheme + "://" + u.Host + "/auth/oauth/token", nil
}

type dattoTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

// MintDattoToken performs the Datto RMM OAuth2 password-grant exchange:
//
//	POST {host}/auth/oauth/token
//	Authorization: Basic base64("public-client:public")
//	Content-Type: application/x-www-form-urlencoded
//	grant_type=password&username=<apiKey>&password=<apiSecretKey>
//
// It returns the access token and its lifetime in seconds (~100h in practice).
func MintDattoToken(ctx context.Context, baseURL, apiKey, apiSecretKey string) (token string, expiresIn int, err error) {
	endpoint, err := dattoTokenEndpoint(baseURL)
	if err != nil {
		return "", 0, err
	}

	form := url.Values{}
	form.Set("grant_type", "password")
	form.Set("username", apiKey)
	form.Set("password", apiSecretKey)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	// "public-client:public" is Datto's fixed, documented public OAuth client
	// (not a secret); it identifies the password-grant client. The user's API
	// key/secret carried in the form body are the real credentials.
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("public-client:public")))

	httpClient := &http.Client{Timeout: 30 * time.Second}
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", 0, fmt.Errorf("requesting Datto RMM token: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return "", 0, fmt.Errorf("Datto RMM token request failed: HTTP %d (check DATTO_RMM_API_KEY / DATTO_RMM_API_SECRET_KEY and that DATTO_RMM_PLATFORM matches your account region)", resp.StatusCode)
	}

	var tr dattoTokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return "", 0, fmt.Errorf("parsing Datto RMM token response: %w", err)
	}
	if tr.AccessToken == "" {
		return "", 0, fmt.Errorf("Datto RMM token response did not contain an access_token")
	}
	return tr.AccessToken, tr.ExpiresIn, nil
}

// EnsureDattoToken makes sure cfg carries a usable bearer token, minting one
// from the API key + secret key when needed. It is a no-op when:
//   - a pre-minted token (DATTO_RMM_TOKEN) is set,
//   - a saved access token is still valid (not within 5 minutes of expiry), or
//   - no API key/secret is available (the API then returns 401 with a clear
//     message rather than the CLI failing to start).
//
// On a fresh mint it persists the token to the config file (best-effort) so
// subsequent invocations reuse it for the token's ~100h lifetime.
func EnsureDattoToken(ctx context.Context, cfg *Config) error {
	if cfg == nil {
		return nil
	}
	// A pre-minted token wins; AuthHeader() uses it directly.
	if cfg.DattoRmmToken != "" {
		return nil
	}
	// A saved access token that is not near expiry is reused as-is.
	if cfg.AccessToken != "" {
		if cfg.TokenExpiry.IsZero() || time.Now().Before(cfg.TokenExpiry.Add(-5*time.Minute)) {
			return nil
		}
	}
	// Need both key and secret to mint a token.
	if cfg.DattoRmmApiKey == "" || cfg.DattoRmmApiSecretKey == "" {
		return nil
	}

	token, expiresIn, err := MintDattoToken(ctx, cfg.BaseURL, cfg.DattoRmmApiKey, cfg.DattoRmmApiSecretKey)
	if err != nil {
		return err
	}
	cfg.AccessToken = token
	if expiresIn > 0 {
		cfg.TokenExpiry = time.Now().Add(time.Duration(expiresIn) * time.Second)
	}
	cfg.AuthSource = "oauth:DATTO_RMM_API_KEY"
	// Persist best-effort; a read-only home dir (CI) must not break the command.
	_ = cfg.SaveTokens(cfg.ClientID, cfg.ClientSecret, token, cfg.RefreshToken, cfg.TokenExpiry)
	return nil
}
