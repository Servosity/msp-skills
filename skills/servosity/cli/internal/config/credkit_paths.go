// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated; handfixes.json credkit-wiring): every file Load
// may read a credential from, for the credential kit's startup discovery
// (internal/credwire). It resolves through Load's own resolveConfigPath and
// the credentials store's own path functions, so the two cannot drift.

package config

import "servosity-msp-pp-cli/internal/cliutil"

// ReadPaths is every credential file Load(configPath) may read: the config
// file, the legacy config, the store beside an explicit config file, and the
// default store. Holding one the program then does not read is safe; missing
// one it reads is not.
func ReadPaths(configPath string) ([]string, error) {
	path, explicit, err := resolveConfigPath(configPath)
	if err != nil {
		return nil, err
	}
	out := []string{path}
	if explicit {
		if store, err := cliutil.CredentialsFilePathForConfig(path); err == nil {
			out = append(out, store)
		}
	}
	if legacy, err := LegacyConfigPath(); err == nil && legacy != "" && legacy != path {
		out = append(out, legacy)
	}
	if store, err := cliutil.CredentialsFilePath(); err == nil {
		out = append(out, store)
	}
	return out, nil
}
