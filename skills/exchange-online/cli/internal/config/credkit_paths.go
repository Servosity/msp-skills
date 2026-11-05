// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated; handfixes.json credkit-wiring): the files Load
// reads, for the credential kit's startup discovery (internal/credwire). It
// resolves through Load's own resolveConfigPath and the credential store's own
// path functions, so the two cannot drift; TestReadPathsCoversEveryFileLoadReads
// traces the reads.

package config

import (
	"os"
	"path/filepath"

	"exchange-online-pp-cli/internal/cliutil"
)

// ReadPaths is the config file Load(configPath) takes its settings from: the
// explicit or environment file, or config.toml in the config directory, or the
// legacy file when that is missing. Its base_url is the run's.
func ReadPaths(configPath string) ([]string, error) {
	path, explicit, err := resolveConfigPath(configPath)
	if err != nil {
		return nil, err
	}
	if !explicit {
		if _, serr := os.Lstat(path); os.IsNotExist(serr) {
			legacy, err := LegacyConfigPath()
			if err != nil {
				return nil, err
			}
			if legacy != path {
				return []string{path, legacy}, nil
			}
		}
	}
	return []string{path}, nil
}

// StorePaths is the credential store beside the config and the global one.
func StorePaths(configPath string) ([]string, error) {
	path, _, err := resolveConfigPath(configPath)
	if err != nil {
		return nil, err
	}
	global, err := cliutil.CredentialsFilePath()
	if err != nil {
		return nil, err
	}
	beside, err := cliutil.CredentialsFilePathForConfig(path)
	if err != nil || beside == "" || beside == global {
		return []string{global}, nil
	}
	return []string{beside, global}, nil
}

// ConsultedPaths is the default config and the legacy one, which Load reads under
// an explicit selection too (trustedGlobalCredentialOrigin judges the global store
// by their base_url) without using anything else in them. So their values are only
// held: declared to the kit, the first base_url found would become the base URL it
// allows credentials to.
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
