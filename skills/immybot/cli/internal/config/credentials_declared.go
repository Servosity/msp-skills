// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-authored (NOT generated; handfixes.json credkit-wiring): every
// credential a loaded config carries, held with the shared credential kit, and
// the files the kit's startup discovery reads (internal/credwire).

package config

import (
	"os"
	"strings"

	"immybot-pp-cli/internal/cliutil"
)

// DeclaredCredential is one credential value a config holds, and the kit
// source it is held under, which decides where it may be sent.
type DeclaredCredential struct {
	Source string
	Value  string
}

// DeclaredCredentials is every credential value this config holds, from
// every source Load merges (the config file including [headers], the
// credentials file, the environment) and every value set on it since.
// Identifiers (client_id, the Entra tenant id, ids, tenant names) are never
// credentials.
func (c *Config) DeclaredCredentials() []DeclaredCredential {
	if c == nil {
		return nil
	}
	out := []DeclaredCredential{
		{"config:client_secret", c.ClientSecret},
		{"config:auth_header", c.AuthHeaderVal},
		{"config:auth_header", c.AuthHeader()},
		{"config:access_token", c.AccessToken},
		{"config:refresh_token", c.RefreshToken},
	}
	for name, value := range c.Headers {
		out = append(out, DeclaredCredential{"header:" + name, value})
	}
	return out
}

// HoldCredentials holds every declared credential and the credentials inside
// the base and token URLs. It runs after every load and every token save, so a
// value acquired while running is held.
func (c *Config) HoldCredentials() {
	if c == nil {
		return
	}
	for _, cred := range c.DeclaredCredentials() {
		cliutil.HoldValues(cred.Source, cred.Value)
	}
	cliutil.HoldURLCredentials(c.BaseURL)
	cliutil.HoldURLCredentials(c.TokenURL)
}

// HoldCredentialsFrom is HoldCredentials for a config decoded from data, plus
// every value data's lines carry where the decode failed.
func (c *Config) HoldCredentialsFrom(data []byte) {
	c.HoldCredentials()
	cliutil.HoldConfigBytes(data)
}

// ReadPaths is every file Load(configPath) may read, for the credential kit's
// startup discovery (internal/credwire): the explicit or environment config
// file, the legacy file, and the credentials store each of them uses. It
// resolves through Load's own resolveConfigPath, so the two cannot drift.
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

// SelectedBaseURL is the API base Load(configPath) routes to with {subdomain}
// filled, or "" when Load fails and nothing will be sent.
func SelectedBaseURL(configPath string) string {
	cfg, err := Load(configPath)
	if err != nil {
		return ""
	}
	return strings.ReplaceAll(cfg.BaseURL, "{subdomain}", cfg.TemplateVars["subdomain"])
}

// HoldProcessCredentials holds what the kit's startup discovery cannot name:
// every value of every config and credentials file Load could read for configPath once --home and a bound
// profile have applied. It applies nothing, writes nothing and prints nothing;
// Load still decides what is used.
func HoldProcessCredentials(configPath string) {
	path, explicit, err := resolveConfigPath(configPath)
	if err != nil {
		return
	}
	paths := []string{path}
	if legacy, err := LegacyConfigPath(); err == nil {
		paths = append(paths, legacy)
	}
	for _, p := range paths {
		_ = readConfigFile(p, &Config{}, "config")
	}
	if p, err := cliutil.CredentialsFilePath(); err == nil {
		cliutil.HoldCredentialsFile(p)
	}
	if explicit {
		if p, err := cliutil.CredentialsFilePathForConfig(path); err == nil {
			cliutil.HoldCredentialsFile(p)
		}
	}
}
