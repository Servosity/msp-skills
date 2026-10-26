// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"net/http"

	"ninjaone-pp-cli/internal/cliutil"
)

// mintClientCredentialsToken is `auth login`'s token exchange with the
// supplied credential held by the secret funnel before the request is built
// and every returned error funnelled, so a transport error that names a
// redirect target made out of the secret is withheld like an echoed body.
// Class token-errors-funnelled-at-source; reapply/token_errors.py renames the
// rendered function to mintClientCredentialsTokenRaw.
func mintClientCredentialsToken(httpClient *http.Client, tokenURL, clientID, clientSecret string) (*tokenResponse, error) {
	cliutil.FunnelHold(clientID, clientSecret)
	tok, err := mintClientCredentialsTokenRaw(httpClient, tokenURL, clientID, clientSecret)
	return tok, cliutil.FunnelError(err, clientID, clientSecret)
}
