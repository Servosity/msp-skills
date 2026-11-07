// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package config

// Hand-written (NOT generated): what the shared credential kit's wiring
// (internal/credwire, handfixes.json credkit-wiring) and the config.toml
// rewrite (config-rewrite-keeps-file-values) need from this package.

import (
	"errors"
	"reflect"
	"strings"

	"github.com/pelletier/go-toml/v2"

	"appdirect-pp-cli/internal/cliutil"
	"net/url"
)

// ErrConfigHoldsCredential marks a config.toml write refused because it would
// hold a credential (an agentcookie-managed save).
var ErrConfigHoldsCredential = errors.New("refusing to write config.toml: it would hold a credential (keep credentials in the environment or the credential store)")

// ResolveConfigPath is the config file Load reads for configPath, and whether
// it was named explicitly (--config or APPDIRECT_CONFIG).
func ResolveConfigPath(configPath string) (string, bool, error) { return resolveConfigPath(configPath) }

// StartupBaseURL is the base URL Load will use under the current home: the
// environment's, else the base_url of the file Load reads (an explicit
// --config or APPDIRECT_CONFIG, else config.toml with its legacy fallback),
// else the built-in one. The kit binds the credentials to this origin.
func StartupBaseURL(configFlag string) string {
	if v := strings.TrimSpace(cliutil.EnvOverride("APPDIRECT_BASE_URL")); v != "" {
		return v
	}
	path, explicit, err := resolveConfigPath(configFlag)
	if err != nil {
		return builtinBaseURL()
	}
	var opened cliutil.OpenedConfig
	if explicit {
		opened, err = cliutil.ReadOpenedConfig(path)
	} else {
		legacy, _ := LegacyConfigPath()
		opened, _, err = openDefaultConfig(path, legacy)
	}
	if err != nil {
		return builtinBaseURL()
	}
	var file struct {
		BaseURL *string `toml:"base_url"`
	}
	if toml.Unmarshal(opened.Data, &file) != nil || file.BaseURL == nil {
		return builtinBaseURL()
	}
	return *file.BaseURL
}

// configFileDiffers reports whether data would change the values config.toml
// at path holds. A file that reads back to the same values is not rewritten.
func configFileDiffers(path string, data []byte) bool {
	old, err := cliutil.ReadOpenedConfig(path)
	if err != nil {
		return true
	}
	var was, will map[string]any
	if toml.Unmarshal(old.Data, &was) != nil || toml.Unmarshal(data, &will) != nil {
		return true
	}
	return !reflect.DeepEqual(was, will)
}

// urlWithoutCredential is raw without the parts a credential rides in (user
// name and password, query, fragment), or fallback when raw does not parse.
func urlWithoutCredential(raw, fallback string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return fallback
	}
	u.User, u.RawQuery, u.ForceQuery, u.Fragment, u.RawFragment = nil, "", false, "", ""
	return u.String()
}
