// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored (handfixes.json: auth-hint-reads-body-not-path), ported from
// skills/connectsecure/cli/internal/cli/helpers.go.

package cli

import (
	"errors"
	"strings"

	"watchguard-pp-cli/internal/client"
)

// authSignalText is the text the HTTP 400 auth heuristic reads: the API error
// body, not the whole message. The message carries the request path, and
// every Firebox user/group path and every AuthPoint path contains
// "/authentication/", so `\bauthenticat` turned any 400 on them (a field
// validation error) into "auth is missing or invalid" with exit 4. A body
// that names a token, API key or authorization stays an auth signal; one that
// reads as a field validation error does not.
func authSignalText(err error) string {
	var apiErr *client.APIError
	if errors.As(err, &apiErr) {
		lower := strings.ToLower(apiErr.Body)
		for _, a := range []string{"token", "api key", "api_key", "apikey", "api-key", "authoriz", "authenticat", "unauthori", "expired"} {
			if strings.Contains(lower, a) {
				return apiErr.Body
			}
		}
		for _, v := range []string{"already exists", "required", "must be", "between", "invalid value", "not valid", "duplicate"} {
			if strings.Contains(lower, v) {
				return ""
			}
		}
		return apiErr.Body
	}
	return err.Error()
}
