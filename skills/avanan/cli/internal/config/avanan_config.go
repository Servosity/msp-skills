// Copyright 2026 geekbrownbear and contributors. Licensed under Apache-2.0. See LICENSE.

// Avanan-specific credential persistence.
//
// The generated SaveCredential writes the application ID (the value
// AuthHeader() resolves). Avanan additionally needs the minted session token
// and the app-id/secret pair persisted independently, so those accessors live
// here rather than as edits to the generated config.

package config

import "time"

// SaveAvananSession persists a minted session token and its expiry.
//
// The token is a short-lived derivative of the app-id/secret pair, so it is
// written alongside the other credential fields and cleared by logout.
func (c *Config) SaveAvananSession(token string, expiry time.Time) error {
	c.AvananToken = token
	c.TokenExpiry = expiry
	delete(c.envOverrides, "AvananToken")
	delete(c.envOverrides, "TokenExpiry")
	c.updateFileConfigField("AvananToken")
	c.updateFileConfigField("TokenExpiry")
	if err := c.saveCredentialsFirst(); err != nil {
		return err
	}
	return c.save()
}

// SaveAvananCredentials persists the application ID and client secret used to
// mint session tokens.
func (c *Config) SaveAvananCredentials(appID, secret string) error {
	if appID != "" {
		c.AvananAppId = appID
		delete(c.envOverrides, "AvananAppId")
		c.updateFileConfigField("AvananAppId")
	}
	if secret != "" {
		c.ClientSecret = secret
		delete(c.envOverrides, "ClientSecret")
		c.updateFileConfigField("ClientSecret")
	}
	if err := c.saveCredentialsFirst(); err != nil {
		return err
	}
	return c.save()
}

// AvananSessionToken returns the persisted session token, if any.
func (c *Config) AvananSessionToken() string {
	if c == nil {
		return ""
	}
	return c.AvananToken
}

// AvananAppID returns the configured application ID.
func (c *Config) AvananAppID() string {
	if c == nil {
		return ""
	}
	return c.AvananAppId
}

// AvananClientSecret returns the configured client secret.
func (c *Config) AvananClientSecret() string {
	if c == nil {
		return ""
	}
	return c.ClientSecret
}
