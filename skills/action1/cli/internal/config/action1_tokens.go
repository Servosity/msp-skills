// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package config

import (
	"strings"
	"time"

	"action1-pp-cli/internal/cliutil"
)

// AccessTokenExpired reports whether the cached minted token is past (or within
// 60s of) its recorded expiry. A missing expiry means "unknown", treated as
// valid. action1 handfix: client-credentials-mint.
func (c *Config) AccessTokenExpired() bool {
	if c.AccessToken == "" || c.TokenExpiry.IsZero() {
		return false
	}
	return time.Now().After(c.TokenExpiry.Add(-60 * time.Second))
}

// StoredClientID and StoredClientSecret are the client pair the configuration
// file and the credential store recorded, before any environment override: the
// pair the stored minted token belongs to (SaveTokens writes the pair with the
// token). "" when the files hold none (action1 handfix
// minted-token-bound-to-its-client; Codex r1 action1 class cached
// authentication ignores credential override). A configuration Load did not
// build (no file snapshot, no environment overrides) is its own record.
func (c *Config) StoredClientID() string {
	if c == nil {
		return ""
	}
	if c.fileConfig == nil {
		return c.ClientID
	}
	return c.fileConfig.ClientID
}

func (c *Config) StoredClientSecret() string {
	if c == nil {
		return ""
	}
	if c.fileConfig == nil {
		return c.ClientSecret
	}
	return c.fileConfig.ClientSecret
}

// tokenRecord is the provenance of the stored token (action1 handfix
// token-owned-by-its-record; Codex r2 action1 class cached authentication
// ignores credential override, --config form): Load merges the config file,
// a --config file's own credentials and the global credential store, so the
// merged fields cannot say which client a token was minted for. The record
// that supplied the token names it, and only that record may cache another.
type tokenRecord struct {
	// tokenClientID is the client ID stored in the same record as the access
	// token; "" when that record named no client (`auth set-token`).
	tokenClientID string
	// tokenKnown is set once a record supplied the token (or SaveTokens wrote
	// one), so tokenClientID is meaningful.
	tokenKnown bool
	// globalClientID is the client ID of the global credential store's record
	// when Load read it; globalRead says it did.
	globalClientID string
	globalRead     bool
	// explicitConfig is set when --config (or ACTION1_CONFIG) named the file.
	explicitConfig bool
}

func (r *tokenRecord) saved(clientID string) {
	r.tokenClientID, r.tokenKnown = strings.TrimSpace(clientID), true
	r.globalClientID, r.globalRead = strings.TrimSpace(clientID), true
}

// bindFileToken records that the config file just parsed supplied the token
// (with its own client ID, or none).
func (c *Config) bindFileToken() {
	c.tokenRecord.tokenClientID, c.tokenRecord.tokenKnown = "", false
	if c.AccessToken != "" {
		c.tokenRecord.tokenClientID, c.tokenRecord.tokenKnown = strings.TrimSpace(c.ClientID), true
	}
}

// takeRecordToken takes the access token from a credential record, with that
// record's client as its owner. (Refresh token and expiry keep the generated
// field-by-field merge: a refresh is only attempted for an owned token, on
// the same API origin.)
func (c *Config) takeRecordToken(creds *cliutil.Credentials) {
	c.AccessToken = creds.AccessToken
	c.tokenRecord.tokenClientID, c.tokenRecord.tokenKnown = strings.TrimSpace(creds.ClientID), true
}

func (c *Config) noteGlobalRecord(creds *cliutil.Credentials) {
	c.tokenRecord.globalClientID, c.tokenRecord.globalRead = strings.TrimSpace(creds.ClientID), true
}

// StoredTokenOwned reports whether the stored access token was minted for the
// client ID in effect: the record that supplied the token named that same
// client. A token with no recorded client (`auth set-token`) or another
// client's (an environment pair, or a --config pair, that names another API
// client) is never sent for this one. A configuration Load did not build is
// its own record.
func (c *Config) StoredTokenOwned() bool {
	if c == nil || c.AccessToken == "" {
		return false
	}
	id := strings.TrimSpace(c.ClientID)
	if id == "" {
		return false
	}
	if c.tokenRecord.tokenKnown {
		return id == c.tokenRecord.tokenClientID
	}
	return c.fileConfig == nil
}

// PairIsStored reports whether the client pair in effect is the one the
// files hold, so a token minted for it may be stored beside it. A pair the
// environment supplies (and the files do not hold) keeps its token in memory:
// stored, it would read as the files' client's token on the next run. The
// token is saved to the global credential store, so a pair is stored there
// only when that store's record is this client's (or, with no --config,
// there is no record yet): a --config pair, or a pair beside another
// client's record, keeps its token in memory and never rewrites that record.
func (c *Config) PairIsStored() bool {
	if c == nil || c.ClientID == "" || c.ClientSecret == "" {
		return false
	}
	if c.ClientID != c.StoredClientID() || c.ClientSecret != c.StoredClientSecret() {
		return false
	}
	r := c.tokenRecord
	if r.globalRead {
		return r.globalClientID == strings.TrimSpace(c.ClientID)
	}
	return !r.explicitConfig
}

// action1StoreScopeIdentity names the local database by the Action1 API client
// (action1 handfix store-scope-follows-client; Codex r1 action1 class rotating
// token changes offline store identity): the default scope hashed the bearer
// token, which a client pair re-mints every hour, so each mint opened another,
// empty database. With no client pair (a pasted token) the generated scope
// stands.
func action1StoreScopeIdentity(c *Config) string {
	if c == nil {
		return ""
	}
	// An explicit token authenticates instead of the pair (the client sends
	// it as is), so the store follows that token, never the stored pair's
	// client (action1 handfix store-follows-the-authenticating-credential;
	// Codex r2 action1 class cross-tenant offline store aliasing).
	if c.AuthHeaderVal != "" || c.Action1Oauth2 != "" {
		return ""
	}
	if id := strings.TrimSpace(c.ClientID); id != "" {
		return "action1-client-id=" + id
	}
	return ""
}

func init() { StoreScopeIdentity = action1StoreScopeIdentity }
