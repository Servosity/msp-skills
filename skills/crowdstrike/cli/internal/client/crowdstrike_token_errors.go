// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"context"
	"sync"
)

// crowdstrike handfix: rejected-mint-returns (the ninjaone shape, fleet
// mint_lock_scan.py). The client-credentials exchange runs with the
// token-state lock held, and maskCredentialText takes that lock itself, so a
// rejected mint whose error masked the token endpoint's answer locked twice
// and hung forever. tools/apply_edits.py renames the rendered function to
// mintClientCredentialsRaw and points its error bodies at shownTokenErrorBody.

// mintClientCredentials runs the token exchange. The caller holds the
// token-state lock, as for mintClientCredentialsRaw. The client pair is held by
// the credential kit from startup (Declarations: FALCON_CLIENT_ID,
// FALCON_CLIENT_SECRET, config:client_id, config:client_secret), and the
// minted token where it is kept (keepMintedToken).
func (c *Client) mintClientCredentials(ctx context.Context, clientID, clientSecret string) error {
	// per-cloud-token-url: no configured, trusted token URL, nothing is
	// posted (the rendered mint returned nil and sent the API request
	// unauthenticated).
	if ResolveCrowdStrikeTokenURL(c.Config) == "" {
		return ErrNoCrowdStrikeTokenURL
	}
	return c.mintClientCredentialsRaw(ctx, clientID, clientSecret)
}

// shownTokenErrorBody is the token exchange's error text: the press's
// credential masks over the answer's display cut, through a view of the same
// Config with a lock of its own. Reading Config unlocked here is safe: the
// caller holds the real lock.
func (c *Client) shownTokenErrorBody(body []byte, extraCredentials ...string) string {
	view := &Client{Config: c.Config, ccMu: &sync.Mutex{}}
	return view.maskCredentialText(truncateBody(body), extraCredentials...)
}
