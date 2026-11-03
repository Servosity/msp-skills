// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package config

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// NSMLogin records which NSM tenant the saved token belongs to. nsm-login
// writes it beside the config file; the store scope below reads it.
type NSMLogin struct {
	Tenant         string `json:"tenant"`
	ProductGroupID string `json:"product_group_id"`
	TenantSerial   string `json:"tenant_serial"`
	Region         string `json:"region"`
	// TokenFingerprint binds this record to the exact token nsm-login saved
	// (a truncated SHA-256, not the token). A different active token means the
	// record does not describe it.
	TokenFingerprint string `json:"token_fingerprint"`
}

func tokenFingerprint(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:8])
}

// nsmLoginPath is per config file (config.toml -> config.nsm-login.json), so two
// configs in one directory never share tenant metadata.
func nsmLoginPath(c *Config) string {
	if c == nil || strings.TrimSpace(c.Path) == "" {
		return ""
	}
	base := filepath.Base(c.Path)
	return filepath.Join(filepath.Dir(c.Path), strings.TrimSuffix(base, filepath.Ext(base))+".nsm-login.json")
}

// LoadNSMLogin returns the tenant recorded by the last nsm-login, if any.
func (c *Config) LoadNSMLogin() (NSMLogin, bool) {
	var l NSMLogin
	path := nsmLoginPath(c)
	if path == "" {
		return l, false
	}
	data, err := os.ReadFile(path) // #nosec G304 -- sibling of the user's own config file (c.Path); MCP callers cannot choose --config or --home.
	if err != nil || json.Unmarshal(data, &l) != nil || l.TenantSerial == "" {
		return NSMLogin{}, false
	}
	return l, true
}

// SaveNSMLogin records the tenant behind a freshly minted token (no secrets:
// only a fingerprint of the token is stored).
func (c *Config) SaveNSMLogin(l NSMLogin, token string) error {
	l.TokenFingerprint = tokenFingerprint(token)
	path := nsmLoginPath(c)
	if path == "" {
		return nil
	}
	data, err := json.MarshalIndent(l, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// The NSM token rotates every 8 hours, and the default local database is
// scoped by credential. When a MySonicWall key is set and the active token is
// the one nsm-login saved for a recorded tenant, pin the scope to that key + NSM
// tenant (hashed by the caller)
// so re-running nsm-login keeps the same store and posture history while
// different keys or tenants never share one. Otherwise defer to the generator's
// credential-derived default, which isolates accounts but restarts history when
// a token-only setup rotates its token.
func init() {
	StoreScopeIdentity = func(c *Config) string {
		key := strings.TrimSpace(c.SonicwallMswApiKey)
		// AuthHeader() sends AuthHeaderVal, then SonicwallNsmToken, then AccessToken;
		// only the last one is what nsm-login saved and fingerprinted.
		if key == "" || c.AuthHeaderVal != "" || c.envOverrides["SonicwallNsmToken"] || c.SonicwallNsmToken != "" || c.AccessToken == "" {
			return "" // the active token is not the one nsm-login saved: keep credential isolation
		}
		login, ok := c.LoadNSMLogin()
		if !ok || login.TokenFingerprint != tokenFingerprint(c.AccessToken) {
			return ""
		}
		return "sonicwall:msw_api_key=" + key + ";nsm_tenant=" + login.TenantSerial + ";region=" + login.Region
	}
}

// SaveNSMToken persists a freshly minted NSM token so it is the one AuthHeader
// sends. A token persisted under nsm_token outranks AccessToken in AuthHeader,
// so the persisted copy is cleared even while SONICWALL_NSM_TOKEN overrides it
// in this process; otherwise unsetting the variable later would resurrect the
// stale file value. shadowed reports that the environment variable still wins.
func (c *Config) SaveNSMToken(token string) (shadowed bool, err error) {
	shadowed = c.envOverrides["SonicwallNsmToken"]
	if c.fileConfig != nil {
		c.fileConfig.SonicwallNsmToken = ""
	}
	if !shadowed {
		c.SonicwallNsmToken = ""
		c.updateFileConfigField("SonicwallNsmToken")
	}
	c.AuthHeaderVal = ""
	c.updateFileConfigField("AuthHeaderVal")
	return shadowed, c.SaveTokens("", "", token, "", time.Time{})
}
