// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package config

import (
	"strings"

	"microsoft-graph-pp-cli/internal/credkit"
)

// holdConfigCredentials holds every credential a loaded configuration carries
// (environment, config file or credential store) with the credential kit
// (handfixes.json credkit-wiring): the credential fields, the Authorization
// header, [headers] values and what the base URL carries, and the same of the
// file layer an environment value overrode. The kit's startup discovery holds
// the files as they were at Start; the MCP server loads the config again on
// every tool call, so a file written since is held here when it is read.
func holdConfigCredentials(c *Config) {
	if c == nil {
		return
	}
	credkit.Hold("config:graph_token", c.MicrosoftGraphToken)
	credkit.Hold("config:access_token", c.AccessToken)
	credkit.Hold("config:refresh_token", c.RefreshToken)
	credkit.Hold("config:client_secret", c.ClientSecret)
	credkit.Hold("config:auth_header", c.AuthHeaderVal, c.AuthHeader())
	for k, v := range c.Headers {
		credkit.Hold(credkit.Source("header:"+strings.ToLower(k)), v)
	}
	credkit.HoldURL("userinfo", c.BaseURL)
	if c.fileConfig != nil && c.fileConfig != c {
		holdConfigCredentials(c.fileConfig)
	}
}
