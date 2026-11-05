// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import "sync"

// shownTokenErrorBody is shownErrorBody for the client-credentials mint. The
// mint runs with the token-state lock held (authHeader), and
// maskCredentialText takes that lock itself, so a rejected mint (a wrong or
// expired client secret) deadlocked. It masks through a view of the same
// Config with a lock of its own; reading Config unlocked is safe because the
// caller holds the real lock. Ported from ninjaone (handfixes.json
// token-errors-funnelled-at-source).
func (c *Client) shownTokenErrorBody(body []byte, extraCredentials ...string) string {
	view := &Client{Config: c.Config, ccMu: &sync.Mutex{}}
	return view.shownErrorBody(body, extraCredentials...)
}
