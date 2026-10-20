// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written (td-synnex): helpers auth mint, set-token and logout call before saving.

package config

// KeepFileEndpoint resets BaseURL and TemplateVars to the values Load read from
// the config file (or the legacy file it fell back to). save() persists the
// in-memory values, so without this a run with TD_SYNNEX_BASE_URL pointed at a
// test server would write that host into config.toml, and every later request
// would send its bearer token there.
func (c *Config) KeepFileEndpoint() {
	if c == nil || c.fileConfig == nil {
		return
	}
	c.BaseURL = c.fileConfig.BaseURL
	c.TemplateVars = cloneStringMap(c.fileConfig.TemplateVars)
}

// DropFileAccessToken clears a file-stored synnex_access_token. AuthHeader ranks
// it above the access token mint or set-token saves, so a stale one would keep
// authenticating requests. A value from the environment keeps its precedence
// for this run, but the stale file copy is still dropped from what gets saved
// (configForSave writes the file snapshot back while an env override is set).
func (c *Config) DropFileAccessToken() {
	if c == nil {
		return
	}
	if c.fileConfig != nil {
		c.fileConfig.TdSynnexAccessToken = ""
	}
	if !c.envOverrides["TdSynnexAccessToken"] {
		c.TdSynnexAccessToken = ""
	}
}
