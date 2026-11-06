// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated; handfixes.json credkit-wiring): the files Load
// reads, for the credential kit's startup discovery (internal/credwire), and
// the hold of what a URL setting carries. ReadPaths resolves through Load's
// own resolveConfigPath and the credential store's own path functions, so the
// two cannot drift.

package config

import (
	"os"
	"slices"

	"crowdstrike-pp-cli/internal/cliutil"
)

// ReadPaths is every file Load(configPath) may read a credential from: the
// explicit or environment file, its profile credential store and the global
// one, else config.toml in the config directory, the legacy file when that
// one does not exist, and the global credential store. The file whose
// base_url decides comes first, as the kit takes the first base_url it reads.
func ReadPaths(configPath string) ([]string, error) {
	path, explicit, err := resolveConfigPath(configPath)
	if err != nil {
		return nil, err
	}
	out := []string{path}
	if explicit {
		if creds, err := cliutil.CredentialsFilePathForConfig(path); err == nil {
			out = append(out, creds)
		}
	} else {
		legacy, err := LegacyConfigPath()
		if err != nil {
			return nil, err
		}
		if _, serr := os.Stat(path); os.IsNotExist(serr) && legacy != "" && legacy != path {
			out = append(out, legacy)
		}
	}
	// Load falls back to the global store when a profile has none.
	if global, err := cliutil.CredentialsFilePath(); err == nil && !slices.Contains(out, global) {
		out = append(out, global)
	}
	return out, nil
}

// holdURLSettings holds the password and every query value of the base and
// token URLs a config file names, with the kit's own reader: the program
// prints a URL setting back in a connection error and in doctor. A file the
// MCP server reads after startup is held here, when it is read.
func holdURLSettings(c *Config) {
	if c == nil {
		return
	}
	for _, v := range []string{c.BaseURL, c.TokenURL} {
		if v != "" {
			cliutil.HoldURL("userinfo", v)
		}
	}
}
