// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated; handfixes.json credkit-wiring): the files Load
// reads, for the credential kit's startup discovery (internal/credwire). It
// resolves through Load's own resolveConfigPath and the credential store's
// own path functions, so the two cannot drift.

package config

import (
	"os"
	"path/filepath"
	"slices"

	"kaseya-vsa-pp-cli/internal/cliutil"
)

// ReadPaths is every file Load(configPath) may read a credential from: the
// explicit or environment file, its profile credential store and the global one, else
// config.toml in the config directory, the legacy file when that one does not
// exist, and the global credential store.
func ReadPaths(configPath string) ([]string, error) {
	path, explicit, err := resolveConfigPath(configPath)
	if err != nil {
		return nil, err
	}
	if explicit {
		out := []string{path}
		if creds, err := cliutil.CredentialsFilePathForConfig(path); err == nil {
			out = append(out, creds)
		}
		// Load falls back to the global store when the profile has none.
		if global, err := cliutil.CredentialsFilePath(); err == nil && !slices.Contains(out, global) {
			out = append(out, global)
		}
		return out, nil
	}
	out := []string{path}
	legacy, err := LegacyConfigPath()
	if err != nil {
		return nil, err
	}
	if _, serr := os.Stat(path); os.IsNotExist(serr) && legacy != "" && legacy != path {
		out = append(out, legacy)
	}
	creds, err := cliutil.CredentialsFilePath()
	if err != nil {
		return nil, err
	}
	return append(out, creds), nil
}

// ConsultedPaths is the default config and the legacy one, which Load reads
// under an explicit selection too (trustedGlobalCredentialOrigin judges the
// global store by their base_url) without using anything else in them. So their
// values are only held: declared to the kit, the first base_url found would
// become the base URL it allows credentials to.
func ConsultedPaths() ([]string, error) {
	dir, err := cliutil.ConfigDir()
	if err != nil {
		return nil, err
	}
	legacy, err := LegacyConfigPath()
	if err != nil {
		return nil, err
	}
	return []string{filepath.Join(dir, "config.toml"), legacy}, nil
}
