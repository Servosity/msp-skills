// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package client

// appdirect handfix minted-token-bound-to-its-client: an access token is sent
// only for the client pair and the token URL it was minted with. Each AppDirect
// marketplace is its own tenant, so an operator switching APPDIRECT_CLIENT_ID /
// APPDIRECT_BASE_URL between marketplaces must never send one marketplace's
// token with the other's pair. The generated mint saved every minted token
// beside the STORED pair (the environment pair itself stays off disk), so the
// next run read an environment pair's token as the stored client's token.
//
//   - A stored token is used only when the files recorded the client ID in
//     effect with it and the token URL resolves the same way.
//   - A pair the files do not hold keeps its token in memory, once per process
//     (the MCP server reuses it until it expires).

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"sync"
	"time"

	"appdirect-pp-cli/internal/config"
	"appdirect-pp-cli/internal/credkit"
)

type processToken struct {
	access string
	expiry time.Time
}

var processTokens sync.Map // pairKey -> processToken

func pairKey(clientID, clientSecret, tokenURL string) string {
	sum := sha256.Sum256([]byte(clientID + "\x00" + clientSecret + "\x00" + tokenURL))
	return hex.EncodeToString(sum[:])
}

// storedTokenOwned reports whether cfg's access token came from the files for
// the client and token URL in effect.
func storedTokenOwned(cfg *config.Config, clientID string) bool {
	if cfg == nil || cfg.AccessToken == "" {
		return false
	}
	s := cfg.StoredLayer()
	if s == nil || s.AccessToken != cfg.AccessToken {
		return false
	}
	id := strings.TrimSpace(clientID)
	tokenURL := ResolveAppDirectTokenURL(cfg) // "" binds nothing: no configured host owns the token
	return id != "" && tokenURL != "" && id == strings.TrimSpace(s.ClientID) && ResolveAppDirectTokenURL(s) == tokenURL
}

// tokenBinding decides the mint for an active client pair: (false, true) when
// this process already holds a live token for the pair (put in effect), (true,
// true) when the token in effect belongs to another pair or URL, and (_, false)
// when no pair is active or the stored token is this pair's (the generated
// expiry rules decide).
func (c *Client) tokenBinding() (mint bool, decided bool) {
	cfg := c.Config
	if cfg == nil {
		return false, false
	}
	id, secret := resolveClientCredentials(cfg)
	if id == "" || secret == "" {
		return false, false
	}
	if v, ok := processTokens.Load(pairKey(id, secret, ResolveAppDirectTokenURL(cfg))); ok {
		t := v.(processToken)
		if t.expiry.IsZero() || time.Until(t.expiry) >= 60*time.Second {
			cfg.AuthHeaderVal = ""
			cfg.AccessToken, cfg.TokenExpiry = t.access, t.expiry
			return false, true
		}
		return true, true
	}
	if storedTokenOwned(cfg, id) {
		return false, false
	}
	return true, true
}

// keepMintedToken puts a freshly minted token in effect. It is saved beside the
// pair only when the credential store holds that pair; otherwise it lives in
// this process. An agentcookie-managed configuration keeps its pair in
// config.toml, which is never a credential store (config-file-holds-no-
// credential refuses that save), so its token stays in this process too.
func (c *Client) keepMintedToken(clientID, clientSecret, access string, expiry time.Time) error {
	credkit.Hold("minted:oauth", access) // Hand-fix (handfixes.json credkit-wiring): a minted token is held before it is used.
	processTokens.Store(pairKey(clientID, clientSecret, ResolveAppDirectTokenURL(c.Config)), processToken{access: access, expiry: expiry})
	c.Config.AuthHeaderVal = "" // force AuthHeader() to use the new AccessToken path
	if c.Config.AgentcookieManagedByExternalStore() {
		c.Config.AccessToken, c.Config.TokenExpiry = access, expiry
		return nil
	}
	if c.Config.PairIsStored() && clientID == c.Config.ClientID && clientSecret == c.Config.ClientSecret {
		return c.Config.SaveTokens(clientID, clientSecret, access, "", expiry)
	}
	c.Config.AccessToken, c.Config.TokenExpiry = access, expiry
	return nil
}
