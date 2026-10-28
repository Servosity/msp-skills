// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated; handfixes.json credkit-startup-config-argv):
// the files Load reads, for the credential kit's startup discovery
// (internal/credwire). It resolves through Load's own resolveConfigPath, so
// the two cannot drift.

package config

import "os"

// ReadPaths is every file Load(configPath) may read, in the order it tries
// them: the explicit or environment file alone, else config.toml in the
// config directory, then the legacy file when that one does not exist.
func ReadPaths(configPath string) ([]string, error) {
	path, explicit, err := resolveConfigPath(configPath)
	if err != nil {
		return nil, err
	}
	if explicit {
		return []string{path}, nil
	}
	legacy, err := LegacyConfigPath()
	if err != nil {
		return nil, err
	}
	if _, serr := os.Stat(path); os.IsNotExist(serr) && legacy != "" && legacy != path {
		return []string{path, legacy}, nil
	}
	return []string{path}, nil
}
