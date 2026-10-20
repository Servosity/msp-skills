// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored: a stable local-store identity for CloudAlly.
//
// The generated default names the local store after a hash of the auth
// header plus the client id and secret. CloudAlly access tokens rotate on
// every refresh, re-login and use-account switch, so each rotation opened an
// empty store: restore history vanished, `restore submit --resume` reported
// no earlier batch and told the operator to start a new one (resending
// restores that may already have been accepted), and synced backup status
// disappeared.
//
// The store is keyed instead by the API origin, the API client id and the
// principal the active token acts as: the partner, one customer account
// reached through use-account, or one company login. None of those rotate,
// and no token or secret is part of the key. The principal matters because
// one API key pair (created in the partner portal) serves partner and
// company logins alike, so origin and client id alone would let a company
// login read rows a partner login had synced for every customer.
//
// The principal comes from auth-context.json, and only while that record's
// token fingerprint matches the active token (auth_login.go). A token the
// record does not describe (CLOUDALLY_ACCESS_TOKEN, set-token, a hand-edited
// credentials file) gets the generated per-token key: isolated, as before.
//
// The first run with a principal key adopts the store named for the current
// token (helpers.go adoptLegacyScopedStore, through StoreScopeLegacyCredential),
// so existing history carries over.

package cli

import (
	"net/url"
	"strings"

	"cloudally-pp-cli/internal/config"
)

func init() {
	config.StoreScopeIdentity = cloudallyStoreScopeIdentity
}

// cloudallyStoreScopeIdentity returns the stable store key, or "" (the
// generated credential-derived key) when there is no client id, the active
// credential is a raw auth header, or no auth record vouches for the active
// token's principal.
func cloudallyStoreScopeIdentity(c *config.Config) string {
	if c == nil {
		return ""
	}
	clientID := strings.TrimSpace(c.ClientID)
	token := strings.TrimSpace(c.AccessToken)
	if clientID == "" || token == "" || strings.TrimSpace(c.AuthHeaderVal) != "" {
		return ""
	}
	actx, ok := loadAuthContext()
	if !ok {
		return ""
	}
	principal := actx.activePrincipal(bindingFor(c), token)
	if principal == "" {
		return ""
	}
	return "cloudally:origin=" + cloudallyStoreOrigin(c.BaseURL) + "|client_id=" + clientID + "|principal=" + principal
}

// cloudallyStoreOrigin normalizes an API base URL: scheme and host lower
// case, no trailing slash, so spelling variants of one URL share a store.
// An unparseable value is used as written.
func cloudallyStoreOrigin(raw string) string {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return raw
	}
	return strings.ToLower(u.Scheme) + "://" + strings.ToLower(u.Host) + strings.TrimRight(u.EscapedPath(), "/")
}
