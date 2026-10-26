// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"context"
	"sync"

	"ninjaone-pp-cli/internal/cliutil"
)

// Class token-errors-funnelled-at-source. The client-credentials exchange is
// the one request whose errors left the client unmasked: a transport error
// names the redirect target, which a token endpoint can make out of the
// secret it was sent. Every error it returns now passes the funnel here, so no
// caller's printer (sync events, warnings, MCP results) can carry it.
// reapply/token_errors.py renames the rendered function to mintClientCredentialsRaw.

// mintClientCredentials runs the token exchange with its errors funnelled. The
// caller holds the token-state lock, as for mintClientCredentialsRaw.
func (c *Client) mintClientCredentials(ctx context.Context, clientID, clientSecret string) error {
	cliutil.FunnelHold(clientID, clientSecret)
	return cliutil.FunnelError(c.mintClientCredentialsRaw(ctx, clientID, clientSecret), clientID, clientSecret)
}

// shownTokenErrorBody is shownErrorBody for the token exchange. The exchange
// runs with the token-state lock held and maskCredentialText takes that lock
// itself, so it masks through a view of the same Config with a lock of its
// own. Reading Config unlocked here is safe: the caller holds the real lock.
func (c *Client) shownTokenErrorBody(body []byte, extraCredentials ...string) string {
	view := &Client{Config: c.Config, ccMu: &sync.Mutex{}}
	return view.shownErrorBody(body, extraCredentials...)
}
