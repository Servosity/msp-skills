// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package config

import (
	"errors"
	"os"

	"kaseya-bms-pp-cli/internal/cliutil"
	"kaseya-bms-pp-cli/internal/credkit"
)

// holdConfigCredentials holds every credential the loaded configuration
// carries with the credential kit (handfixes.json credkit-wiring): the kit
// read the files at startup, and this covers a configuration loaded from
// anywhere else (a test, an MCP tool's own Load). Then it applies the token
// binding to the loaded configuration (dropKbmsTokenOutsideItsBinding); the
// values it drops stay held.
func holdConfigCredentials(c *Config) {
	holdConfigValues(c)
	dropKbmsTokenOutsideItsBinding(c) // kaseya-bms handfix: minted-token-bound-to-login-and-host, at Load
}

func holdConfigValues(c *Config) {
	if c == nil {
		return
	}
	credkit.Hold("config:client_secret", c.ClientSecret)
	credkit.Hold("config:access_token", c.AccessToken)
	credkit.Hold("config:refresh_token", c.RefreshToken)
	credkit.Hold("config:bms_token", c.KaseyaBmsToken)
	credkit.Hold("config:bms_bearer_auth", c.KaseyaBmsBearerAuth)
	credkit.Hold("config:auth_header", c.AuthHeaderVal)
	for k, v := range c.Headers {
		credkit.Hold("header:"+k, v)
	}
	credkit.HoldURL("userinfo", c.BaseURL) // the kit's own source for a base URL's password; "config:base_url" is an Identifier
	KbmsEnvLogin()                         // holds the login password
	if c.fileConfig != nil && c.fileConfig != c {
		holdConfigValues(c.fileConfig)
	}
}

// CredentialFiles is every file Load may read a credential from under the
// current home, in the order Load reads them: the config file it resolves
// (KASEYA_BMS_CONFIG or the default path), the legacy config path when the
// default one is missing (Load falls back to it only then), the credential
// store beside an explicit config, and the global credential store
// (credkit Declarations.Files, internal/credwire).
func CredentialFiles() []string {
	path, explicit, err := resolveConfigPath("")
	if err != nil {
		return nil
	}
	out := []string{path}
	if explicit {
		if p, err := cliutil.CredentialsFilePathForConfig(path); err == nil {
			out = append(out, p)
		}
	} else if legacy, err := LegacyConfigPath(); err == nil {
		if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
			out = append(out, legacy)
		}
	}
	if p, err := cliutil.CredentialsFilePath(); err == nil {
		out = append(out, p)
	}
	return out
}
