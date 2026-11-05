// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): handfixes.json credkit-wiring.

package config

import (
	"os"

	"github.com/pelletier/go-toml/v2"

	"hubspot-pp-cli/internal/cliutil"
)

// HoldCredentials holds with the credential kit every credential value c
// carries: the auth header, tokens, client id and secret, every [headers]
// value, and the file's own values before env overrides. It runs after every
// load and every token save, so a value acquired while running is held.
func (c *Config) HoldCredentials() {
	for _, cfg := range []*Config{c, c.fileConfigOrNil()} {
		if cfg == nil {
			continue
		}
		cliutil.HoldCredential("config:auth_header", cfg.AuthHeaderVal)
		cliutil.HoldCredential("config:access_token", cfg.AccessToken)
		cliutil.HoldCredential("config:refresh_token", cfg.RefreshToken)
		cliutil.HoldCredential("config:client_id", cfg.ClientID)
		cliutil.HoldCredential("config:client_secret", cfg.ClientSecret)
		for name, v := range cfg.Headers {
			cliutil.HoldCredential("header:"+name, v)
		}
	}
}

func (c *Config) fileConfigOrNil() *Config {
	if c == nil {
		return nil
	}
	return c.fileConfig
}

// ReadPaths is every file Load(configPath) may read, for the credential kit's
// startup discovery (internal/credwire): the explicit or environment config
// file, config.toml in the config directory, the legacy file, and the
// credentials store each of them uses. It resolves through Load's own
// resolveConfigPath, so the two cannot drift.
func ReadPaths(configPath string) []string {
	var out []string
	if path, explicit, err := resolveConfigPath(configPath); err == nil {
		out = append(out, path)
		if explicit {
			if stored, err := cliutil.CredentialsFilePathForConfig(path); err == nil {
				out = append(out, stored)
			}
		}
	}
	if legacy, err := LegacyConfigPath(); err == nil && legacy != "" {
		if _, serr := os.Stat(legacy); serr == nil {
			out = append(out, legacy)
		}
	}
	if stored, err := cliutil.CredentialsFilePath(); err == nil {
		out = append(out, stored)
	}
	return out
}

// BaseURLFile is the one file Load("") takes its base URL from: the environment
// file, else config.toml in the config directory, else the legacy file when it
// parses. Load's base URL comes from no other file; a credentials store's
// base_url records where a token was issued, nothing more. The kit itself takes
// the base URL from an explicit --config.
func BaseURLFile() string {
	path, explicit, err := resolveConfigPath("")
	if err != nil || explicit {
		return path
	}
	legacy, err := LegacyConfigPath()
	if err != nil {
		return path
	}
	opened, source, err := openDefaultConfig(path, legacy)
	if err == nil && source != path && parseConfigData(opened.Data, &Config{}, source, "legacy config path") != nil {
		return ""
	}
	return source
}

// HoldFile holds the credentials of a regular file the program may not read,
// and names no base URL: the same fields the kit holds from a file it reads, so
// a setting there cannot refuse a start, or every string when it does not parse.
func HoldFile(path string) {
	data := cliutil.ReadRegular(path)
	var c Config
	if err := toml.Unmarshal(data, &c); err != nil {
		cliutil.HoldConfigBytes(data)
		return
	}
	c.HoldCredentials()
	cliutil.HoldBaseURL(c.BaseURL)
}
