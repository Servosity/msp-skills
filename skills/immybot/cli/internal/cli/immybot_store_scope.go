// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored. Kept in its own file so `generate --force` preserves it.
//
// The local store holds one ImmyBot instance's data, so its scope key names
// the instance. The base URL is the template https://{subdomain}.immy.bot
// (spec.json servers[0].variables.subdomain), and the press's stable key
// hashes that literal template: two instances signed in with one Entra app
// registration shared one mirror, and one instance's computers answered for
// the other. Every key here names the base URL resolved through the template
// variables, the way the response cache already keys on the subdomain, and a
// store kept from before that key is never adopted or claimed.

package cli

import (
	"strings"

	"immybot-pp-cli/internal/client"
	"immybot-pp-cli/internal/config"
)

func init() { config.StoreScopeIdentity = immyStoreScopeIdentity }

// immyStoreNeverClaims keeps defaultDBPathInDir (helpers.go) from claiming a
// data.db kept from before the instance key: whose instance it holds is
// unknown, so it is left where it is and the instance's own store is used.
var immyStoreNeverClaims = true

// immyUnknownInstanceScope keys the store of a run whose instance is unknown
// (its config failed to load, or no scope was set): a file no known instance
// ever opens, so that run reads no instance's data.
const immyUnknownInstanceScope = "instance=unknown"

// immyStoreScopeIdentity names the instance in every key: with the press's
// stable identity (client id, secret, refresh token, tenant id) when there is
// one, else with the legacy credential (a token-only login), else alone (a
// logged-out CLI), so no two instances ever share a store.
func immyStoreScopeIdentity(c *config.Config) string {
	var identity []string
	if c.ClientID != "" {
		identity = append(identity, "client_id="+c.ClientID)
	}
	if c.ClientSecret != "" {
		identity = append(identity, "client_secret="+c.ClientSecret)
	}
	if c.RefreshToken != "" {
		identity = append(identity, "refresh_token="+c.RefreshToken)
	}
	if c.ImmybotTenantId != "" && c.ImmybotTenantId != c.AccessToken {
		identity = append(identity, "tenant_id="+c.ImmybotTenantId)
	}
	if len(identity) == 0 {
		if legacy := strings.TrimSpace(c.StoreScopeLegacyCredential()); legacy != "" {
			identity = []string{legacy}
		}
	}
	base := strings.ToLower(strings.TrimRight(strings.TrimSpace(client.ResolvedBaseURL(c.BaseURL, c.TemplateVars)), "/"))
	return strings.Join(append([]string{"instance=" + base}, identity...), "\n")
}
