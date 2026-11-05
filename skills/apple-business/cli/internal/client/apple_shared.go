// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: lets fan-out workers (one client each) share one rate limiter,
// and marks token-exchange failures so callers never treat them as an
// unreachable API.

package client

import (
	"errors"
)

// ErrTokenExchange wraps a failure to reach the OAuth token endpoint.
var ErrTokenExchange = errors.New("token exchange failed")

// ShareLimiter makes c pace through from's limiter, so --rate-limit and 429
// back-off stay one budget across parallel worker clients.
func (c *Client) ShareLimiter(from *Client) {
	if from != nil && from.limiter != nil {
		c.limiter = from.limiter
	}
}

// shownTokenAnswer is the token endpoint's answer as a mint or refresh error
// may show it: this client's credentials masked as maskCredentialText masks
// them. The credential kit's transport has already held what the answer carries
// and every sink checks it. It runs where the token-state lock is already held
// (authHeader, the 401 re-mint), so the client's credentials are read here and
// passed in, and the masker runs without a Config: it would take the same lock
// again, and that mutex is not reentrant. extra keeps every mask the press error
// path applied (the mint passes the client ID and secret).
func (c *Client) shownTokenAnswer(body []byte, extra ...string) string {
	if c != nil && c.Config != nil {
		cfg := c.Config
		extra = append(extra, cfg.AuthHeaderVal, cfg.AuthHeader(), cfg.AccessToken, cfg.RefreshToken, cfg.ClientSecret, cfg.AppleBusinessPrivateKey)
	}
	return (&Client{}).maskCredentialText(string(body), extra...)
}
