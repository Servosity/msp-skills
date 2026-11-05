// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated; handfixes.json credkit-wiring): the files Load
// reads, for the credential kit's startup discovery (internal/credwire). It
// resolves through Load's own resolveConfigPath and the credential store's
// own path functions, so the two cannot drift.

package config

import (
	"os"
	"slices"

	"warranty-pp-cli/internal/cliutil"
)

// ReadPaths is every file Load(configPath) may read a credential from, split in
// two. files are the config files Load takes base_url from: the explicit or
// environment file, else config.toml in the config directory and the legacy file
// when that one does not exist. stores are the credential stores it falls back to:
// the profile store and the global one. A store's base_url is the server a
// credential was saved for, never the base URL a run uses.
func ReadPaths(configPath string) (files, stores []string, err error) {
	path, explicit, err := resolveConfigPath(configPath)
	if err != nil {
		return nil, nil, err
	}
	files = []string{path}
	if explicit {
		if creds, err := cliutil.CredentialsFilePathForConfig(path); err == nil {
			stores = append(stores, creds)
		}
	} else {
		legacy, err := LegacyConfigPath()
		if err != nil {
			return nil, nil, err
		}
		if _, serr := os.Stat(path); os.IsNotExist(serr) && legacy != "" && legacy != path {
			files = append(files, legacy)
		}
	}
	// Load falls back to the global store when the profile has none.
	global, err := cliutil.CredentialsFilePath()
	if err != nil {
		if explicit {
			return files, stores, nil
		}
		return nil, nil, err
	}
	if !slices.Contains(stores, global) {
		stores = append(stores, global)
	}
	return files, stores, nil
}
