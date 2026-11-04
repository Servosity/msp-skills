// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

// nerdio handfix minted-token-bound-to-its-client: an access token is sent
// only for the client pair and the token URL it was minted with. Each Nerdio
// NMM instance is its own tenant, so an operator switching NERDIO_CLIENT_ID /
// NERDIO_BASE_URL between NMM instances must never send one NMM instance's
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
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"nerdio-pp-cli/internal/config"
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
	tokenURL := ResolveNerdioTokenURL(cfg) // "" binds nothing: no configured host owns the token
	if issuedTo, ok := tokenIssuedTo(cfg.AccessToken); ok && !strings.EqualFold(issuedTo, id) {
		return false // the token names another client (any failure signal wins)
	}
	return id != "" && tokenURL != "" && id == strings.TrimSpace(s.ClientID) && ResolveNerdioTokenURL(s) == tokenURL
}

// tokenIssuedTo is the client an Entra access token says it was issued to
// (appid in v1 tokens, azp in v2), read from its payload without verifying
// it: a second ownership signal beside the files (Codex review r1 class token
// bound to another client pair), so a store that already holds another
// client's token beside a pair (written before the save gate,
// config.ErrTokenPairNotStored) is never sent for that pair. Two claims that
// disagree name no single client: "" with ok true, which matches no client.
// ok is false for a token that is not a JWT or names no client.
func tokenIssuedTo(access string) (string, bool) {
	parts := strings.Split(strings.TrimSpace(access), ".")
	if len(parts) != 3 {
		return "", false
	}
	payload, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(parts[1], "="))
	if err != nil {
		return "", false
	}
	var claims map[string]any
	if json.Unmarshal(payload, &claims) != nil {
		return "", false
	}
	var names []string
	for _, k := range []string{"appid", "azp"} {
		if v, present := claims[k]; present {
			s, _ := v.(string)
			names = append(names, strings.TrimSpace(s))
		}
	}
	if len(names) == 0 {
		return "", false
	}
	for _, n := range names[1:] {
		if !strings.EqualFold(n, names[0]) {
			return "", true
		}
	}
	return names[0], true
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
	if v, ok := processTokens.Load(pairKey(id, secret, ResolveNerdioTokenURL(cfg))); ok {
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
	processTokens.Store(pairKey(clientID, clientSecret, ResolveNerdioTokenURL(c.Config)), processToken{access: access, expiry: expiry})
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
