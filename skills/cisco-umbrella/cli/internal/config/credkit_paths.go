// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated; handfixes.json credkit-wiring): the files Load
// reads, for the credential kit's startup discovery (internal/credwire). It
// resolves through Load's own resolveConfigPath and the credential store's own
// path functions, so the two cannot drift; TestReadPathsCoversEveryFileLoadReads
// traces the reads.

package config

import (
	"path/filepath"
	"slices"

	"cisco-umbrella-pp-cli/internal/cliutil"
)

// ReadPaths is every file Load(configPath) may read a credential from, the file
// the command names first: the explicit or environment file or config.toml in
// the config directory, config.toml in the config directory again (an explicit
// config's base URL is judged against it), the legacy file, the credential store
// beside the config, and the global one. Holding a file a run does not load is
// safe; missing the one it loads is not, so nothing here looks at what exists.
func ReadPaths(configPath string) ([]string, error) {
	path, _, err := resolveConfigPath(configPath)
	if err != nil {
		return nil, err
	}
	out := []string{path}
	add := func(p string) {
		if p != "" && !slices.Contains(out, p) {
			out = append(out, p)
		}
	}
	dir, err := cliutil.ConfigDir()
	if err != nil {
		return nil, err
	}
	add(filepath.Join(dir, "config.toml"))
	legacy, err := LegacyConfigPath()
	if err != nil {
		return nil, err
	}
	add(legacy)
	creds, err := cliutil.CredentialsFilePathForConfig(path)
	if err == nil {
		add(creds)
	}
	global, err := cliutil.CredentialsFilePath()
	if err != nil {
		return nil, err
	}
	add(global)
	return out, nil
}
