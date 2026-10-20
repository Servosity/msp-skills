// Hand-written for saas-alerts (not generated): Respond API access-token mint.
//
// The Respond API (paths under /respondApi/) authenticates with an `accesstoken`
// header, minted by POST /respondApi/api/v1/auth with the partner `api_key`
// header (Respond spec 2.0.0, securitySchemes accessToken + /auth). Without that
// header the API answers 400 "Only secure origins are allowed", which is the
// error browser and n8n users hit. doInternal attaches the token to every
// Respond request and re-mints once when the API rejects it.

package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
)

const (
	respondPathPrefix = "/respondApi/"
	respondAuthPath   = "/respondApi/api/v1/auth"
)

// ponytail: one process-wide token per (base URL, api key); the mutex is held
// across the mint so concurrent callers share one POST /auth.
var respondTokens = struct {
	sync.Mutex
	m map[string]string
}{m: map[string]string{}}

// respondMinted records every token this process has minted, so a token that was
// replaced by a re-mint is still masked if a later error echoes it. Separate lock:
// the mint masks its own errors while holding respondTokens.
var respondMinted = struct {
	sync.Mutex
	all []string
}{}

// respondTokensForMask lists every minted Respond token; client.go's
// maskCredentialText masks them in every error, URL, and log path.
func respondTokensForMask() []string {
	respondMinted.Lock()
	defer respondMinted.Unlock()
	return append([]string(nil), respondMinted.all...)
}

func isRespondPath(path string) bool {
	return strings.HasPrefix(path, respondPathPrefix) && path != respondAuthPath
}

// respondTokenRejected reports the Respond API's answers to a missing, invalid,
// or expired access token (live receipts 2026-10-14: 400 "Only secure origins are
// allowed" without the header, 401 "Access token is invalid" with a bad one).
func respondTokenRejected(status int, body []byte) bool {
	s := string(body)
	return (status == http.StatusBadRequest && strings.Contains(s, "Only secure origins are allowed")) ||
		(status == http.StatusUnauthorized && strings.Contains(s, "Access token"))
}

// respondAccessToken returns the cached Respond access token for this client's
// base URL and API key, minting one when none is cached or refresh is true.
func (c *Client) respondAccessToken(ctx context.Context, apiKey string, refresh bool) (string, error) {
	if apiKey == "" {
		return "", fmt.Errorf("Respond API calls need SAAS_ALERTS_API_KEY: the CLI exchanges it for a Respond access token via POST %s", respondAuthPath)
	}
	key := c.BaseURL + "\x00" + apiKey
	respondTokens.Lock()
	defer respondTokens.Unlock()
	if tok := respondTokens.m[key]; tok != "" && !refresh {
		return tok, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+respondAuthPath, nil)
	if err != nil {
		return "", c.respondMintErr("creating Respond auth request", err, apiKey)
	}
	req.Header.Set("api_key", apiKey)
	req.Header.Set("Accept", "application/json")
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", c.respondMintErr("minting Respond access token", err, apiKey)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", c.respondMintErr("reading Respond auth response", err, apiKey)
	}
	if resp.StatusCode >= 400 {
		return "", &APIError{
			Method:     http.MethodPost,
			Path:       respondAuthPath,
			StatusCode: resp.StatusCode,
			Body:       c.funnelText(truncateBody(c.respondMintDisplay(body, apiKey)), apiKey) + " (Respond access-token mint; check SAAS_ALERTS_API_KEY and that Respond is enabled for the partner)",
		}
	}
	var out struct {
		AccessToken string `json:"accessToken"`
	}
	if err := json.Unmarshal(body, &out); err != nil || out.AccessToken == "" {
		return "", fmt.Errorf("Respond auth (POST %s) returned no accessToken", respondAuthPath)
	}
	respondTokens.m[key] = out.AccessToken
	if len(out.AccessToken) >= 8 { // ponytail: shorter strings are not real tokens and would mangle masked text
		respondMinted.Lock()
		respondMinted.all = append(respondMinted.all, out.AccessToken)
		respondMinted.Unlock()
	}
	return out.AccessToken, nil
}

// respondMintErr masks credentials in a mint failure; when masking changes nothing
// the original error stays wrapped so callers can still detect network errors.
func (c *Client) respondMintErr(label string, err error, apiKey string) error {
	if masked := c.maskError(err, apiKey).Error(); masked != err.Error() { // saas-alerts hand-fix: error-text-secret-funnel
		return fmt.Errorf("%s: %s", label, masked)
	}
	return fmt.Errorf("%s: %w", label, err)
}

// respondMintDisplay is the text of a failed mint answer, withheld whole when
// it quotes back the partner key or a Respond token (error-text-secret-funnel).
func (c *Client) respondMintDisplay(body []byte, apiKey string) []byte {
	if c.echoesCredential(body, apiKey) {
		return []byte(echoWithheldText)
	}
	return body
}
