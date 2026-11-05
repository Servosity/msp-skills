// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"context"
	"sync"

	"appdirect-pp-cli/internal/cliutil"
)

// appdirect handfix: rejected-mint-returns (the ninjaone shape, fleet
// mint_lock_scan.py). The client-credentials exchange runs with the
// token-state lock held, and maskCredentialText takes that lock itself, so a
// rejected mint whose error masked the token endpoint's answer locked twice
// and hung forever. tools/apply_edits.py renames the rendered function to
// mintClientCredentialsRaw and points its error bodies at shownTokenErrorBody.

// mintClientCredentials runs the token exchange with the client ID and secret
// held, and its error withheld if it repeats either. The caller holds the
// token-state lock, as for mintClientCredentialsRaw.
func (c *Client) mintClientCredentials(ctx context.Context, clientID, clientSecret string) error {
	cliutil.FunnelHold(clientID, clientSecret)
	// per-marketplace-token-url: no configured, trusted token URL, nothing is
	// posted (the rendered mint returned nil and sent the API request
	// unauthenticated).
	if ResolveAppDirectTokenURL(c.Config) == "" {
		return ErrNoAppDirectTokenURL
	}
	err := c.mintClientCredentialsRaw(ctx, clientID, clientSecret)
	if err == nil {
		return nil
	}
	if shown := cliutil.FunnelWithhold(err.Error(), []string{clientID, clientSecret}); shown != err.Error() {
		return withheldTokenError{msg: shown, err: err}
	}
	return err
}

// withheldTokenError keeps the wrapped error for errors.Is classification
// while its text is the withheld one.
type withheldTokenError struct {
	msg string
	err error
}

func (e withheldTokenError) Error() string { return e.msg }
func (e withheldTokenError) Unwrap() error { return e.err }

// shownTokenErrorBody is shownErrorBody for the token exchange: it masks
// through a view of the same Config with a lock of its own. Reading Config
// unlocked here is safe: the caller holds the real lock.
func (c *Client) shownTokenErrorBody(body []byte, extraCredentials ...string) string {
	view := &Client{Config: c.Config, ccMu: &sync.Mutex{}}
	return view.shownErrorBody(body, extraCredentials...)
}
