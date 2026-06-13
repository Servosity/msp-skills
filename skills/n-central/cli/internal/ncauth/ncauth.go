// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

// Package ncauth implements N-central's two-step authentication: a long-lived
// User-API JWT (generated in the N-central UI) is exchanged via
// POST /api/auth/authenticate for a short-lived access token (default 1h) plus
// a refresh token (default ~25h). Data endpoints reject the long-lived JWT;
// they require the exchanged access token. This package keeps a valid access
// token in the Config, refreshing or re-exchanging as needed.
package ncauth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"n-central-pp-cli/internal/cliutil"
	"n-central-pp-cli/internal/config"
)

// authResponse mirrors the body of POST /api/auth/authenticate and
// POST /api/auth/refresh.
type authResponse struct {
	Tokens struct {
		Access struct {
			Token         string `json:"token"`
			Type          string `json:"type"`
			ExpirySeconds int    `json:"expirySeconds"`
		} `json:"access"`
		Refresh struct {
			Token         string `json:"token"`
			Type          string `json:"type"`
			ExpirySeconds int    `json:"expirySeconds"`
		} `json:"refresh"`
	} `json:"tokens"`
}

// expirySkew is subtracted from the recorded expiry so a token that is about to
// expire mid-request is refreshed proactively.
const expirySkew = 60 * time.Second

// authLimiter paces the auth handshake (exchange / refresh / validate) so a
// burst of commands started together cannot hammer N-central's auth endpoint.
// It starts conservatively — auth calls are infrequent (roughly one per token
// lifetime) — and adapts down on 429, mirroring the data client's limiter.
var authLimiter = cliutil.NewAdaptiveLimiter(5.0)

func httpClient() *http.Client { return &http.Client{Timeout: 30 * time.Second} }

func authBase(cfg *config.Config) string {
	return strings.TrimRight(cfg.BaseURL, "/")
}

// Ensure guarantees cfg holds a valid (non-expired) access token, performing a
// refresh or a full JWT exchange when necessary. It is a no-op when a fresh
// access token is already present, and a no-op in printing-press verify/mock
// mode (the mock server does not implement the auth handshake).
func Ensure(ctx context.Context, cfg *config.Config) error {
	if cfg == nil {
		return nil
	}
	if os.Getenv("PRINTING_PRESS_VERIFY") == "1" {
		return nil
	}
	// A valid, unexpired access token needs no work.
	if cfg.AccessToken != "" && !cfg.TokenExpiry.IsZero() && time.Now().Before(cfg.TokenExpiry.Add(-expirySkew)) {
		return nil
	}
	// Prefer a cheap refresh when we have a refresh token.
	if cfg.RefreshToken != "" {
		if err := refresh(ctx, cfg); err == nil {
			return nil
		}
		// fall through to a full exchange on any refresh failure
	}
	return exchange(ctx, cfg)
}

// exchange performs the initial JWT -> access/refresh token exchange.
func exchange(ctx context.Context, cfg *config.Config) error {
	jwt := strings.TrimSpace(cfg.NcentralJwt)
	if jwt == "" {
		return fmt.Errorf("no N-central JWT configured: set NCENTRAL_JWT or run 'n-central-cli auth set-token <jwt>'")
	}
	url := authBase(cfg) + "/auth/authenticate"
	resp, err := postBearer(ctx, url, jwt, nil)
	if err != nil {
		return fmt.Errorf("authenticating to N-central: %w", err)
	}
	return applyTokens(cfg, resp)
}

// refresh mints a new access token from the stored refresh token.
func refresh(ctx context.Context, cfg *config.Config) error {
	url := authBase(cfg) + "/auth/refresh"
	// N-central's refresh token has type "Body"; send it as the request body.
	// Some builds also accept it as a Bearer header, so we send both shapes:
	// the body carries the token, and we also set the Authorization header.
	body := map[string]string{"refreshToken": cfg.RefreshToken}
	resp, err := postBearer(ctx, url, cfg.RefreshToken, body)
	if err != nil {
		return err
	}
	return applyTokens(cfg, resp)
}

// authMaxRetries bounds how many times postBearer retries a 429 on the auth
// handshake before surfacing a typed cliutil.RateLimitError.
const authMaxRetries = 3

// postBearer POSTs to url with an Authorization: Bearer <token> header and an
// optional JSON body, returning the decoded auth response. A 429 from the auth
// endpoint is honored per its Retry-After header (capped by cliutil.MaxRetryWait)
// for a bounded number of attempts, then surfaced as a typed
// cliutil.RateLimitError so callers can distinguish throttling from a hard
// auth failure rather than treating it as an opaque "HTTP 429".
func postBearer(ctx context.Context, url, bearer string, body any) (*authResponse, error) {
	var payload []byte
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		payload = b
	}

	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+bearer)
		req.Header.Set("Accept", "application/json")
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		authLimiter.Wait()
		hresp, err := httpClient().Do(req)
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return nil, ctxErr
			}
			return nil, err
		}

		// Rate limited: pace down, honor Retry-After for a bounded number of
		// attempts, then surface a typed error so throttling is never mistaken
		// for a hard auth rejection or for "no data".
		if hresp.StatusCode == http.StatusTooManyRequests {
			authLimiter.OnRateLimit()
			wait := cliutil.RetryAfter(hresp)
			if attempt < authMaxRetries {
				bodyBytes, _ := io.ReadAll(hresp.Body)
				hresp.Body.Close()
				_ = bodyBytes
				if sleepErr := sleepContext(ctx, wait); sleepErr != nil {
					return nil, sleepErr
				}
				continue
			}
			bodyBytes, _ := io.ReadAll(hresp.Body)
			hresp.Body.Close()
			return nil, &cliutil.RateLimitError{
				URL:        url,
				RetryAfter: wait,
				Body:       strings.TrimSpace(string(bodyBytes)),
			}
		}
		authLimiter.OnSuccess()

		if hresp.StatusCode == http.StatusUnauthorized || hresp.StatusCode == http.StatusForbidden {
			hresp.Body.Close()
			return nil, fmt.Errorf("N-central rejected the token (HTTP %d) — the JWT may be expired, the API user's password may have expired (default 90 days, silently invalidates the JWT), or MFA is enabled on the API user", hresp.StatusCode)
		}
		if hresp.StatusCode >= 400 {
			hresp.Body.Close()
			return nil, fmt.Errorf("auth request to %s failed: HTTP %d", url, hresp.StatusCode)
		}
		dec := json.NewDecoder(hresp.Body)
		var out authResponse
		decErr := dec.Decode(&out)
		hresp.Body.Close()
		if decErr != nil {
			return nil, fmt.Errorf("decoding auth response: %w", decErr)
		}
		if out.Tokens.Access.Token == "" {
			return nil, fmt.Errorf("auth response from %s contained no access token", url)
		}
		return &out, nil
	}
}

// sleepContext sleeps for d or returns early if the context is cancelled.
func sleepContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func applyTokens(cfg *config.Config, resp *authResponse) error {
	expSecs := resp.Tokens.Access.ExpirySeconds
	if expSecs <= 0 {
		expSecs = 3600
	}
	expiry := time.Now().Add(time.Duration(expSecs) * time.Second)
	refreshTok := resp.Tokens.Refresh.Token
	if refreshTok == "" {
		refreshTok = cfg.RefreshToken // keep any existing refresh token
	}
	// Persist to the config file (best effort) and update the in-memory copy so
	// the very next request uses the access token.
	_ = cfg.SaveTokens("", "", resp.Tokens.Access.Token, refreshTok, expiry)
	cfg.AccessToken = resp.Tokens.Access.Token
	cfg.RefreshToken = refreshTok
	cfg.TokenExpiry = expiry
	return nil
}

// Validate calls POST /api/auth/validate with the current access token and
// reports whether it is accepted. Returns nil when valid.
func Validate(ctx context.Context, cfg *config.Config) error {
	if err := Ensure(ctx, cfg); err != nil {
		return err
	}
	url := authBase(cfg) + "/auth/validate"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(nil))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.AccessToken)
	req.Header.Set("Accept", "application/json")
	authLimiter.Wait()
	hresp, err := httpClient().Do(req)
	if err != nil {
		return err
	}
	defer hresp.Body.Close()
	if hresp.StatusCode == http.StatusTooManyRequests {
		authLimiter.OnRateLimit()
		return &cliutil.RateLimitError{URL: url, RetryAfter: cliutil.RetryAfter(hresp)}
	}
	authLimiter.OnSuccess()
	if hresp.StatusCode >= 400 {
		return fmt.Errorf("access token validation failed: HTTP %d", hresp.StatusCode)
	}
	return nil
}
