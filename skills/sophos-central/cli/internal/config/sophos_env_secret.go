// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: pinned by skills/sophos-central/handfixes.json under
// "env-client-secret-not-persisted" (the fleet's issue #266 shape).

package config

// MarkClientSecretFromEnv records that the client secret in memory came from
// SOPHOS_CENTRAL_CLIENT_SECRET, so a save after an automatic token mint keeps
// the file's value instead of writing the environment secret to disk. The
// client ID is still saved: the tenant router ties a cached token to it.
func (c *Config) MarkClientSecretFromEnv() {
	c.markEnvOverride("ClientSecret")
}
