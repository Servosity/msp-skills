// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: pinned by skills/sophos-central/handfixes.json under
// "env-client-secret-not-persisted" (the fleet's issue #266 shape) and
// "env-credential-wins-over-saved".

package config

import (
	"strings"
	"time"

	"sophos-central-pp-cli/internal/cliutil"
)

// MarkClientSecretFromEnv records that the client secret in memory came from
// SOPHOS_CENTRAL_CLIENT_SECRET, so a save after an automatic token mint keeps
// the file's value instead of writing the environment secret to disk. The
// client ID is still saved: the tenant router ties a cached token to it.
func (c *Config) MarkClientSecretFromEnv() {
	c.markEnvOverride("ClientSecret")
}

// KeepEnvSecretOffDisk is MarkClientSecretFromEnv for `auth login`: when the
// login switches to another client ID, the secret saved for the previous one
// is dropped from the file too, so the file never pairs one credential's ID
// with another's secret.
func (c *Config) KeepEnvSecretOffDisk(clientID string) {
	if c.fileConfig != nil && c.fileConfig.ClientID != clientID {
		c.fileConfig.ClientSecret = ""
	}
	c.markEnvOverride("ClientSecret")
}

// tokenInMemoryOnlyMarker is an envOverrides key no field uses; it marks a
// Config whose minted token must not be saved and goes away with the Config.
const tokenInMemoryOnlyMarker = "sophos:token-in-memory-only"

// AdoptEnvClientCredential makes SOPHOS_CENTRAL_CLIENT_ID the credential this
// process uses when it names a different API credential than the client ID
// saved in credentials.toml. The saved secret and the token minted for the
// saved credential are not used (a read-only credential in the environment
// must never send an admin credential's cached token), and the token minted
// for the environment credential is kept in memory so it does not overwrite
// the saved one.
func (c *Config) AdoptEnvClientCredential() {
	envID := strings.TrimSpace(cliutil.EnvOverride("SOPHOS_CENTRAL_CLIENT_ID"))
	if envID == "" || c.ClientID == "" || c.ClientID == envID {
		return
	}
	c.ClientID, c.ClientSecret = envID, ""
	c.AccessToken, c.RefreshToken, c.TokenExpiry = "", "", time.Time{}
	c.markEnvOverride(tokenInMemoryOnlyMarker)
}

// KeepMintedTokenInMemory stores a minted token on c without saving it and
// reports true when c is running on an adopted environment credential.
func (c *Config) KeepMintedTokenInMemory(accessToken string, expiry time.Time) bool {
	if !c.envOverrides[tokenInMemoryOnlyMarker] {
		return false
	}
	c.AccessToken, c.TokenExpiry = accessToken, expiry
	return true
}
