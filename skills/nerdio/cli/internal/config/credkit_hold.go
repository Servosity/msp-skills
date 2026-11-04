// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package config

// Hand-written (handfixes.json credkit-wiring). Load holds every credential
// the configuration it returns carries, with the shared credential kit: the
// MCP server loads the configuration again on every tool call, and a token
// another process saved after this one started was never one of Start's files.

import (
	"nerdio-pp-cli/internal/cliutil"

	"github.com/pelletier/go-toml/v2"
)

// holdConfigCredentials holds c's credentials and those of the file layer an
// environment value overrode, each under the source the kit binds its origins
// to (internal/credwire).
func holdConfigCredentials(c *Config) {
	for cfg, seen := c, 0; cfg != nil && seen < 2; cfg, seen = cfg.fileConfig, seen+1 {
		cliutil.HoldValues("config:client_id", cfg.ClientID)
		cliutil.HoldValues("config:client_secret", cfg.ClientSecret)
		cliutil.HoldValues("config:access_token", cfg.AccessToken)
		cliutil.HoldValues("config:refresh_token", cfg.RefreshToken)
		cliutil.HoldAuthHeader("config:auth_header", cfg.AuthHeaderVal)
		for name, v := range cfg.Headers {
			cliutil.HoldValues("header:"+name, v)
		}
		cliutil.HoldURLCredentials("userinfo", cfg.BaseURL)
		cliutil.HoldURLCredentials("userinfo", cfg.TokenURL)
	}
}

// SelectedBaseURL is the base URL Load routes to for configPath (the --config
// value, or ""), read the way Load reads it but without holding, printing or
// recording anything: internal/credwire runs it inside credkit.Start, before
// any output, so the kit's base_url (every credential's allowed origin) is the
// one this run will use.
func SelectedBaseURL(configPath string) string {
	const placeholder = "https://nmm.example.com" // Load's default
	if v := cliutil.EnvOverride("NERDIO_BASE_URL"); v != "" {
		return v
	}
	path, explicit, err := resolveConfigPath(configPath)
	if err != nil {
		return placeholder
	}
	var opened cliutil.OpenedConfig
	if explicit {
		opened, err = cliutil.ReadOpenedConfig(path)
	} else if legacy, lerr := LegacyConfigPath(); lerr != nil {
		err = lerr
	} else {
		opened, _, err = openDefaultConfig(path, legacy)
	}
	var file struct {
		BaseURL *string `toml:"base_url"`
	}
	if err != nil || toml.Unmarshal(opened.Data, &file) != nil || file.BaseURL == nil {
		return placeholder
	}
	return *file.BaseURL
}
