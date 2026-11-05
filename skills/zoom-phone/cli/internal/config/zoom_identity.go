// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package config

// ZoomPersistAccountID makes an explicit `auth login` save the account ID it
// minted with, so the saved profile stays one consistent identity (app, secret,
// token, account) even when the inputs came from the environment.
func (c *Config) ZoomPersistAccountID() {
	if c.envOverrides != nil {
		delete(c.envOverrides, "ZoomPhoneAccountId")
	}
	if c.fileConfig != nil {
		c.fileConfig.ZoomPhoneAccountId = c.ZoomPhoneAccountId
	}
}

// ZoomFileAccountID returns the account ID saved in the config / credentials
// files, ignoring environment overrides.
func (c *Config) ZoomFileAccountID() string {
	if c.fileConfig == nil {
		return ""
	}
	return c.fileConfig.ZoomPhoneAccountId
}
