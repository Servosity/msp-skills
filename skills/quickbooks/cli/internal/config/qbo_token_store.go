// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (issue #492). Not generated. Recorded hand-fix
// `qbo-token-refresh` in handfixes.json.

package config

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"quickbooks-pp-cli/internal/cliutil"
)

// QBOReload reads the configuration again exactly as the Load that made c did
// (the same --config argument, so the same files, trust checks and
// environment). The token refresh re-reads under its cross-process lock what
// another process may have saved since this one started.
func (c *Config) QBOReload() (*Config, error) {
	if c == nil {
		return nil, errors.New("no configuration loaded")
	}
	return Load(c.qboLoadArg)
}

// QBOStoredToken is the access token and expiry the configuration and
// credential files hold, ignoring QUICKBOOKS_ACCESS_TOKEN: the token a
// refresh saved.
func (c *Config) QBOStoredToken() (string, time.Time) {
	if c == nil {
		return "", time.Time{}
	}
	if c.fileConfig != nil {
		return c.fileConfig.AccessToken, c.fileConfig.TokenExpiry
	}
	return c.AccessToken, c.TokenExpiry
}

// QBOAdoptTokens points this configuration at a new access token, refresh
// token and expiry, in memory, the way SaveTokens leaves them: each value is
// held by the credential kit first, and the environment's access token no
// longer overrides the new one.
func (c *Config) QBOAdoptTokens(access, refresh string, expiry time.Time) {
	if c == nil {
		return
	}
	cliutil.HoldCredential("config:access_token", access) // Hand-fix (handfixes.json credkit-wiring)
	cliutil.HoldCredential("config:refresh_token", refresh)
	c.AccessToken = access
	c.RefreshToken = refresh
	c.TokenExpiry = expiry
	cliutil.QBOHoldTOMLCredentials(c)
	delete(c.envOverrides, "AccessToken")
	delete(c.envOverrides, "RefreshToken")
	delete(c.envOverrides, "TokenExpiry")
	c.updateFileConfigField("AccessToken")
	c.updateFileConfigField("RefreshToken")
	c.updateFileConfigField("TokenExpiry")
}

// QBOSaveRefreshedTokens stores the tokens a refresh returned, beside the
// OAuth client id and secret the store already holds (a client secret set
// only in the environment is not copied to disk). The
// credentials file is written first, durably (temporary file in the same
// directory, 0600, fsync, rename). Unlike SaveTokens it never puts the
// earlier credentials file back when the config file cannot be rewritten:
// QuickBooks rotates refresh tokens, so the earlier file may hold one Intuit
// no longer honors. The config file is rewritten only when it still carries
// credential fields that would shadow the credentials file on the next Load,
// or, for an agentcookie-managed config, because it is where the tokens live.
func (c *Config) QBOSaveRefreshedTokens(access, refresh string, expiry time.Time) error {
	if c == nil {
		return errors.New("no configuration loaded")
	}
	c.AuthHeaderVal = ""
	c.QBOAdoptTokens(access, refresh, expiry)
	if c.AgentcookieManagedByExternalStore() {
		c.markAgentcookieManaged()
		return c.save()
	}
	storePath, err := c.qboTokenStorePath()
	if err != nil {
		return err
	}
	persisted := c.configForSave()
	creds := persisted.credentials()
	shadows := c.qboConfigShadowsStore()
	if shadows {
		// One atomic write says both things: the new tokens, and which config
		// they supersede, so a crash before the rewrite below loses neither.
		creds.SupersedesConfig = qboConfigDigest(c.qboConfigSourcePath())
	}
	if err := cliutil.QBOSaveCredentialsTo(storePath, creds); err != nil {
		return err
	}
	c.CredentialSource = "credentials file"
	if shadows {
		if err := c.save(); err != nil {
			return fmt.Errorf("the new tokens are in the credentials file, but %s still holds older credentials and could not be rewritten: %w", c.Path, err)
		}
	}
	return nil
}

// qboConfigShadowsStore reports whether a config file still holds credential
// fields, which Load prefers over the credentials file.
func (c *Config) qboConfigShadowsStore() bool {
	if c.legacySourcePath != "" && c.legacySourcePath != c.Path {
		return true
	}
	has, err := FileHasCredentialFields(c.Path)
	if err != nil {
		return !errors.Is(err, os.ErrNotExist)
	}
	return has
}

// qboConfigSourcePath is the file this configuration's credential fields were
// read from.
func (c *Config) qboConfigSourcePath() string {
	if c.legacySourcePath != "" {
		return c.legacySourcePath
	}
	return c.Path
}

// qboConfigDigest is the SHA-256 of a config file, or "" when it cannot be read.
func qboConfigDigest(path string) string {
	data, err := os.ReadFile(filepath.Clean(path)) // #nosec G304 -- the config path this program loaded.
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// qboStoreSupersedesConfig reports whether the credentials file a refresh
// saves to names this exact config file as older than itself. An edited config
// has another digest, so it is a new choice and keeps the precedence Load
// gives it.
func (c *Config) qboStoreSupersedesConfig(explicit bool, configPath string) bool {
	stored := cliutil.QBOStoredSupersedes(configPath, explicit)
	return stored != "" && stored == qboConfigDigest(c.qboConfigSourcePath())
}

// qboTokenStorePath is the credentials file a refresh writes: the one Load
// applied credentials from; with none (the credentials came from the
// environment or a config file), the one the next Load looks in first: a
// --config profile's own data/credentials.toml, else the global file.
func (c *Config) qboTokenStorePath() (string, error) {
	if c.qboCredentialsFile != "" {
		return c.qboCredentialsFile, nil
	}
	if _, explicit, err := resolveConfigPath(c.qboLoadArg); err == nil && explicit && c.Path != "" {
		return cliutil.CredentialsFilePathForConfig(c.Path)
	}
	return cliutil.CredentialsFilePath()
}

// qboWithWriterLock runs a write by SaveTokens or ClearTokens under the
// refresh lock of the store it writes (qboTokenStorePath, or the config file of
// an agentcookie-managed store), so the write, and any snapshot it restores on
// failure, never interleaves with a token refresh saving a rotated pair to the
// same store.
func (c *Config) qboWithWriterLock(fn func() error) error {
	lockPath, err := c.QBOTokenLockPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o700); err != nil {
		return err
	}
	return cliutil.QBOWithTokenLock(lockPath, fn)
}

// QBOTokenLockPath is the path the token refresh locks (cliutil.QBOWithTokenLock
// adds ".lock"): beside the credential store the refresh writes, so every
// process that shares the store shares the lock.
func (c *Config) QBOTokenLockPath() (string, error) {
	if c == nil {
		return "", errors.New("no configuration loaded")
	}
	if c.AgentcookieManagedByExternalStore() {
		if c.Path == "" {
			return "", errors.New("no config path")
		}
		return c.Path + ".refresh", nil
	}
	p, err := c.qboTokenStorePath()
	if err != nil {
		return "", err
	}
	return p + ".refresh", nil
}

// EffectiveConfigPath is the one config file Load takes settings from for this
// --config path, as openDefaultConfig chooses it: explicit, or the primary
// default, or the legacy file when no primary exists. "" when none resolves.
func EffectiveConfigPath(configPath string) string {
	path, explicit, err := resolveConfigPath(configPath)
	if err != nil {
		return ""
	}
	if !explicit {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			if legacy, lerr := LegacyConfigPath(); lerr == nil {
				if _, serr := os.Stat(legacy); serr == nil {
					return legacy
				}
			}
		}
	}
	return path
}

// qboRealmBaseURL composes the company base URL from QUICKBOOKS_REALM_ID and
// QUICKBOOKS_ENVIRONMENT, or is "" when no realm is set. Hand-fix (handfixes.json
// qbo-realm-env-base-url).
func qboRealmBaseURL() string {
	realm := strings.TrimSpace(cliutil.EnvOverride("QUICKBOOKS_REALM_ID"))
	if realm == "" {
		return ""
	}
	host := "https://quickbooks.api.intuit.com"
	switch strings.ToLower(strings.TrimSpace(cliutil.EnvOverride("QUICKBOOKS_ENVIRONMENT"))) {
	case "sandbox", "sbx":
		host = "https://sandbox-quickbooks.api.intuit.com"
	}
	return host + "/v3/company/" + url.PathEscape(realm)
}

// SelectedBaseURL is the base URL Load settles on for this --config path, when
// QUICKBOOKS_BASE_URL does not name one: the realm's, then the config in use, then
// the built-in. A config whose permissions Load distrusts chooses nothing, since
// Load will not send a stored credential to the base URL it names. It reads quietly
// and only that one file, so it can run before any output exists.
func SelectedBaseURL(configPath string) string {
	if base := qboRealmBaseURL(); base != "" {
		return base
	}
	path := EffectiveConfigPath(configPath)
	if fi, err := os.Stat(path); err != nil || !fi.Mode().IsRegular() {
		return builtinBaseURL()
	}
	opened, err := cliutil.ReadOpenedConfig(path)
	if err != nil || opened.PermErr != nil {
		return builtinBaseURL()
	}
	if base := baseURLFromConfigBytes(opened.Data); base != "" {
		return base
	}
	return builtinBaseURL()
}

// ReadPaths is every file a configuration with this --config path can read
// credentials from, for credkit.Declarations.Files: the config file, the
// legacy config if one exists, and the credentials store beside each. A reprint
// keeps resolveConfigPath, so the kit reads exactly what Load reads. Hand-fix
// (handfixes.json credkit-wiring).
func ReadPaths(configPath string) []string {
	var out []string
	path, explicit, err := resolveConfigPath(configPath)
	if err == nil {
		out = append(out, path)
		if explicit {
			if store, serr := cliutil.CredentialsFilePathForConfig(path); serr == nil {
				out = append(out, store)
			}
		}
	}
	if legacy, lerr := LegacyConfigPath(); lerr == nil {
		if _, serr := os.Stat(legacy); serr == nil {
			out = append(out, legacy)
		}
	}
	if store, serr := cliutil.CredentialsFilePath(); serr == nil {
		out = append(out, store)
	}
	return out
}
