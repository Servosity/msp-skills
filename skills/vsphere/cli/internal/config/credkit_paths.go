// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated; handfixes.json credkit-wiring): the files Load
// reads, for the credential kit's startup discovery (internal/credwire). It
// resolves through Load's own resolveConfigPath and the credential store's
// own path functions, so the two cannot drift.

package config

import (
	"io"
	"os"
	"path/filepath"
	"slices"

	"vsphere-pp-cli/internal/cliutil"
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

// HoldBaseURLs holds what the base_url of each file in paths carries
// (cliutil.HoldConfigBaseURLs), for the credential kit's startup discovery. It
// reads a regular file only, from the descriptor it judged, so a FIFO or a
// device named as a config cannot stall startup; the kit itself fails closed
// on a file that is not regular.
func HoldBaseURLs(paths []string) {
	for _, path := range paths {
		f, err := os.Open(filepath.Clean(path))
		if err != nil {
			continue
		}
		if info, err := f.Stat(); err == nil && info.Mode().IsRegular() {
			if data, err := io.ReadAll(io.LimitReader(f, maxHeldConfigBytes)); err == nil {
				cliutil.HoldConfigBaseURLs(data)
			}
		}
		_ = f.Close()
	}
}

// maxHeldConfigBytes bounds what HoldBaseURLs reads of one file; a config is a
// few hundred bytes.
const maxHeldConfigBytes = 8 << 20
