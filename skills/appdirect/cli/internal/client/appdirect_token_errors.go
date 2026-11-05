// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"context"
	"sync"
)

// appdirect handfix: rejected-mint-returns (the ninjaone shape, fleet
// mint_lock_scan.py). The client-credentials exchange runs with the
// token-state lock held, and maskCredentialText takes that lock itself, so a
// rejected mint whose error masked the token endpoint's answer locked twice
// and hung forever. tools/apply_edits.py renames the rendered function to
// mintClientCredentialsRaw and points its error bodies at shownTokenErrorBody.

// mintClientCredentials runs the token exchange. The caller holds the
// token-state lock, as for mintClientCredentialsRaw. The client ID and secret
// are held by the shared credential kit from their declared sources.
func (c *Client) mintClientCredentials(ctx context.Context, clientID, clientSecret string) error {
	// per-marketplace-token-url: no configured, trusted token URL, nothing is
	// posted (the rendered mint returned nil and sent the API request
	// unauthenticated).
	if ResolveAppDirectTokenURL(c.Config) == "" {
		return ErrNoAppDirectTokenURL
	}
	return c.mintClientCredentialsRaw(ctx, clientID, clientSecret)
}

// shownTokenErrorBody is the token endpoint's answer as an error shows it,
// masked by the press's own masks through a view of the same Config with a
// lock of its own. Reading Config unlocked here is safe: the caller holds the
// real lock.
func (c *Client) shownTokenErrorBody(body []byte, extraCredentials ...string) string {
	view := &Client{Config: c.Config, ccMu: &sync.Mutex{}}
	return view.maskCredentialText(truncateBody(body), extraCredentials...)
}
