// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: key the local store on the identity a client acts as.

package cli

import (
	"strings"

	"jamf-pro-pp-cli/internal/config"
	"jamf-pro-pp-cli/internal/jamfauth"
)

// The local store is keyed on the same identity every client acts as
// (assess, which answers with the established process identity): one
// server and one API client, stable across token mints and secret rotation.
// A pasted token (auth set-token) adds the token, so two pasted tokens on
// one server do not share a mirror. Only when the config and environment
// hold no credential of any kind does it return "", so data.db is used:
// nothing can be synced without a credential. The generated store path
// never adopts another store for a scoped identity (hand-fix in helpers.go).
// When the identity is refused the key names the refusal, which opens an
// empty store instead of another identity's data.
func init() {
	config.StoreScopeIdentity = func(c *config.Config) string {
		if !jamfauth.PairFromEnv() && !hasAnyCredential(c) {
			return ""
		}
		a, err := assess(c, c.BaseURL)
		if err != nil {
			return "refused\n" + err.Error()
		}
		if a.id.ClientID() == "" && a.keepToken {
			return a.id.Key() + "\ntoken=" + firstSet(c.AuthHeaderVal, c.AccessToken)
		}
		return a.id.Key()
	}
}

// hasAnyCredential mirrors the generated raw store credential: any cached
// header, token, pair or env-loaded client field.
func hasAnyCredential(c *config.Config) bool {
	return firstSet(c.AuthHeaderVal, c.AccessToken, c.RefreshToken, c.ClientID, c.ClientSecret, c.JamfProClientId, c.JamfProClientSecret) != ""
}

func firstSet(v ...string) string {
	for _, s := range v {
		if s = strings.TrimSpace(s); s != "" {
			return s
		}
	}
	return ""
}
