// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"context"
	"sync"

	"nerdio-pp-cli/internal/cliutil"
)

// nerdio handfix: rejected-mint-returns (the ninjaone shape, fleet
// mint_lock_scan.py). The client-credentials exchange runs with the
// token-state lock held, and maskCredentialText takes that lock itself, so a
// rejected mint whose error masked the token endpoint's answer locked twice
// and hung forever. tools/apply_edits.py renames the rendered function to
// mintClientCredentialsRaw and points its error bodies at shownTokenErrorBody.

// mintClientCredentials runs the token exchange with the client ID and secret
// held (handfixes.json credkit-wiring: a pair from a profile or the store is
// held here even when no declared source carried it). The caller holds the
// token-state lock, as for mintClientCredentialsRaw.
func (c *Client) mintClientCredentials(ctx context.Context, clientID, clientSecret string) error {
	cliutil.HoldValues("minted:client-pair", clientID, clientSecret)
	// token-url-from-operator-only / oauth-scope-required
	// (nerdio_token_url.go): no configured, trusted token URL or no scope,
	// nothing is posted.
	if ResolveNerdioTokenURL(c.Config) == "" {
		return ErrNoNerdioTokenURL
	}
	if ResolveNerdioScope() == "" {
		return ErrNoNerdioScope
	}
	return c.mintClientCredentialsRaw(ctx, clientID, clientSecret)
}

// shownTokenErrorBody is shownErrorBody for the token exchange: it masks
// through a view of the same Config with a lock of its own. Reading Config
// unlocked here is safe: the caller holds the real lock.
func (c *Client) shownTokenErrorBody(body []byte, extraCredentials ...string) string {
	view := &Client{Config: c.Config, ccMu: &sync.Mutex{}}
	return view.shownErrorBody(body, extraCredentials...)
}
