// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package config

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
)

// zoom-phone hand-fix: zoom-phone-store-scope-full-identity (handfixes.json
// zoom-phone-store-scope-account-id).
//
// The local store is keyed by the credential actually attached to requests.
//   - A token this CLI minted for (account, app) keys the stable scope: the
//     API host, the Zoom account and the Server-to-Server app, all three, so
//     two accounts behind one app or one account reached through two apps
//     never share rows, and the scope survives every re-mint and secret
//     rotation. "Minted" is proven, not assumed: both mints record a hash
//     binding the token to the account and app (ZoomRecordMint, saved with
//     the token), and only a token whose record matches the current account
//     and app counts.
//   - Any other attached credential (a raw auth_header, an auth set-token
//     token, an access_token with no matching mint record) is keyed by a
//     sha256 of the credential itself, independent of every client_id,
//     phone_client_id or account alias beside it, so a config naming app A
//     and account A with tenant B's token never opens A's store (Codex review
//     r1 of the onboard, class store-scope-identity).
//   - That credential beside app credentials able to mint is never sent: the
//     client replaces it with a mint before its first request
//     (ZoomMintReplacesCredential), so the stable scope applies from the start
//     and no mint can swap the identity under an open store (Codex review r2,
//     mint-transition).
//   - A config [headers] Authorization would override whatever the scope was
//     computed from, so the client refuses it (ZoomRefuseAuthorizationOverride,
//     Codex review r2, header-override).
//   - With nothing attached yet, the next request mints for the profile's
//     account and app, so the stable scope applies (an environment session
//     and the profile it saves share one store).
//   - Conflicting app aliases select neither app's store (ZoomAppIdentity);
//     an identity missing its account or its app gets its own scope, which
//     no complete identity shares.
func init() {
	StoreScopeIdentity = func(c *Config) string {
		base := "base_url=" + normalizeStoreScopeBaseURL(c.BaseURL)
		saved, phone := strings.TrimSpace(c.ClientID), strings.TrimSpace(c.ZoomPhoneClientId)
		app, conflict := c.ZoomAppIdentity()
		account := strings.TrimSpace(c.ZoomPhoneAccountId)
		if conflict {
			// Neither app's store: the token belongs to one app and the
			// other alias names another, so no tenant's rows may be read
			// or written under it. API calls are refused by the CLI's
			// tenant guard (ZoomAppIdentity).
			return base + "\nzoom_app_conflict=" + saved + "\n" + phone
		}
		if cred := c.zoomUnboundCredential(account, app); cred != "" && !c.ZoomMintReplacesCredential() {
			sum := sha256.Sum256([]byte(cred))
			return base + "\nzoom_credential=" + hex.EncodeToString(sum[:])
		}
		switch {
		case app == "" && account == "":
			return ""
		case app == "" || account == "":
			return base + "\nzoom_incomplete_account=" + account + "\nzoom_incomplete_app=" + app
		}
		return base + "\nzoom_account=" + account + "\nzoom_app=" + app
	}
}

// zoomUnboundCredential is the credential the next request attaches when this
// CLI did not mint it for (account, app): a raw auth header always (no mint
// sets one), or "Bearer <access_token>" when the token has no matching mint
// record. Empty when the attached token was minted for (account, app), or when
// nothing is attached yet.
func (c *Config) zoomUnboundCredential(account, app string) string {
	if c == nil {
		return ""
	}
	if v := strings.TrimSpace(c.AuthHeaderVal); v != "" {
		return v
	}
	tok := strings.TrimSpace(c.AccessToken)
	if tok == "" {
		return ""
	}
	if c.zoomMintedFor(account, app, tok) {
		return ""
	}
	return "Bearer " + tok
}

// ZoomCanMint reports that this profile holds the app credentials a token mint
// needs, read the way the client's resolveClientCredentials reads them.
func (c *Config) ZoomCanMint() bool {
	if c == nil {
		return false
	}
	id, secret := c.ClientID, c.ClientSecret
	if id == "" {
		id = os.Getenv("ZOOM_PHONE_CLIENT_ID")
	}
	if secret == "" {
		secret = os.Getenv("ZOOM_PHONE_CLIENT_SECRET")
	}
	return id != "" && secret != ""
}

// ZoomMintReplacesCredential reports that the attached credential is not one
// this CLI minted for the profile's account and app, and that the profile can
// mint its own. The client then mints before its first request, and the store
// scope is the minted one. The two read this one decision so they cannot
// disagree about which credential a run uses.
func (c *Config) ZoomMintReplacesCredential() bool {
	if c == nil {
		return false
	}
	app, conflict := c.ZoomAppIdentity()
	account := strings.TrimSpace(c.ZoomPhoneAccountId)
	if conflict || app == "" || account == "" || !c.ZoomCanMint() {
		return false
	}
	return c.zoomUnboundCredential(account, app) != ""
}

// ZoomRefuseAuthorizationOverride refuses a config [headers] Authorization. The
// request loop would apply it over the minted or saved credential, so every
// call would act as a tenant the store scope and the tenant guard never saw.
func (c *Config) ZoomRefuseAuthorizationOverride() error {
	if c == nil {
		return nil
	}
	for name := range c.Headers {
		if strings.EqualFold(strings.TrimSpace(name), "Authorization") {
			return fmt.Errorf("the config file's [headers] table sets Authorization, which would replace the credential this profile authenticates with; " +
				"remove it, and use auth_header or 'zoom-phone-cli auth set-token' to run with another token")
		}
	}
	return nil
}

// zoomMintRecord binds a minted token to the account and app it was minted
// for. It is a one-way hash, so it reveals nothing of the token.
func zoomMintRecord(account, app, token string) string {
	sum := sha256.Sum256([]byte("zoom-phone-mint-v1\x00" + strings.TrimSpace(account) + "\x00" + strings.TrimSpace(app) + "\x00" + strings.TrimSpace(token)))
	return hex.EncodeToString(sum[:])
}

func (c *Config) zoomMintedFor(account, app, token string) bool {
	if account == "" || app == "" || token == "" || c.ZoomMintRecord == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(c.ZoomMintRecord), []byte(zoomMintRecord(account, app, token))) == 1
}

// ZoomRecordMint records that this CLI minted accessToken for (account, app).
// Both mints (auth login and the client's automatic mint) call it before
// SaveTokens, which persists it with the token.
func (c *Config) ZoomRecordMint(account, app, accessToken string) {
	if c == nil {
		return
	}
	c.ZoomMintRecord = zoomMintRecord(account, app, accessToken)
	c.updateFileConfigField("ZoomMintRecord")
}

// ZoomAppIdentity is the one Zoom app this configuration authenticates as,
// used for the store scope and the tenant guard alike. The profile can name
// the app twice: client_id (what minting uses; auth login and accounts connect
// save it) and phone_client_id (ZOOM_PHONE_CLIENT_ID, or the config file key).
// When both are set and differ the identity is ambiguous: conflict is true and
// app is empty, and every caller fails closed.
func (c *Config) ZoomAppIdentity() (app string, conflict bool) {
	if c == nil {
		return "", false
	}
	saved, phone := strings.TrimSpace(c.ClientID), strings.TrimSpace(c.ZoomPhoneClientId)
	switch {
	case saved != "" && phone != "" && saved != phone:
		return "", true
	case saved != "":
		return saved, false
	}
	return phone, false
}
