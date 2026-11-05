// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): every credential a Config holds, whichever path
// loaded it, goes to the shared credential kit (credkit-wiring).

package config

import (
	"os"

	"github.com/pelletier/go-toml/v2"

	"superops-pp-cli/internal/cliutil"
)

// nonSecretConfigFields are the Config fields that never hold a credential.
// Every other field is one, whatever a reprint names it (fail-closed): its
// strings are held at any length. BaseURL gives only its userinfo, and the
// [headers] table every value but the cliutil non-secret header names
// (cliutil.CredentialStrings).
var nonSecretConfigFields = []string{
	"CACert", "TLSServerName", "AuthSource", "CredentialSource", "AgentcookieManaged",
	"CredentialRefusals", "untrustedCredentialRoute", "configOwner", "legacySourcePath",
	"TokenExpiry", "Path", "envOverrides", "Subdomain", "Region",
}

// CredentialValues is every credential c holds, from any source (config file
// sections including [headers], credentials file, environment), and the copy
// of the config file's own values (fileConfig).
func (c *Config) CredentialValues() []string {
	if c == nil {
		return nil
	}
	return cliutil.CredentialStrings(c, nonSecretConfigFields...)
}

// holdCredentials gives CredentialValues to the kit. Load
// and parseConfigData defer it, so it runs on every return, error returns
// included, before a caller can print anything.
func (c *Config) holdCredentials() {
	cliutil.HoldValues(c.CredentialValues()...)
}

// ReadPaths is every file Load may read a credential from for this --config
// value: the selected config file, the legacy config path and the credentials
// store beside each. credwire.Declarations.Files lists them, and they stay
// outside the kit's file sink as credential sources.
func ReadPaths(configPath string) []string {
	var out []string
	add := func(path string, err error) {
		if err == nil && path != "" {
			out = append(out, path)
		}
	}
	path, _, err := resolveConfigPath(configPath)
	add(path, err)
	add(cliutil.CredentialsFilePathForConfig(path))
	add(LegacyConfigPath())
	add(cliutil.CredentialsFilePath())
	return out
}

// SelectedBaseURL is the base URL Load will route this run to: the file Load
// reads, the environment over it, then the region. Empty when that cannot be
// told before Load reports why. It reads only those two non-secret keys, of a
// file ReadPaths lists.
func SelectedBaseURL(configPath string) string {
	path, explicit, err := resolveConfigPath(configPath)
	if err != nil {
		return ""
	}
	var opened cliutil.OpenedConfig
	if explicit {
		opened, err = cliutil.ReadOpenedConfig(path)
	} else {
		legacy, legacyErr := LegacyConfigPath()
		if legacyErr != nil {
			return ""
		}
		opened, _, err = openDefaultConfig(path, legacy)
	}
	if err != nil && !os.IsNotExist(err) {
		return ""
	}
	file := struct {
		BaseURL string `toml:"base_url"`
		Region  string `toml:"region"`
	}{BaseURL: builtinBaseURL()}
	if err == nil && toml.Unmarshal(opened.Data, &file) != nil {
		return ""
	}
	if v := cliutil.EnvOverride("SUPEROPS_BASE_URL"); v != "" {
		file.BaseURL = v
	}
	if v := os.Getenv("SUPEROPS_REGION"); v != "" {
		file.Region = v
	}
	return routedBaseURL(file.BaseURL, file.Region)
}
