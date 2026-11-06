// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated). Pinned by handfixes.json (credkit-wiring).
// credkit.Start holds every credential file as it is at startup; a file read
// again later (each MCP tool call loads the config afresh) may carry a value
// written since, so every decode holds what it read before anything uses it.

package config

import "windows-server-pp-cli/internal/credkit"

func holdConfigCredentials(c *Config) {
	credkit.Hold("config:server_password", c.WindowsServerPassword)
	credkit.Hold("config:access_token", c.AccessToken)
	credkit.Hold("config:refresh_token", c.RefreshToken)
	credkit.Hold("config:client_secret", c.ClientSecret)
	credkit.Hold("config:auth_header", c.AuthHeaderVal)
	for k, v := range c.Headers {
		credkit.Hold("header:"+k, v)
	}
	credkit.HoldURL("config:base_url", c.BaseURL)
}
