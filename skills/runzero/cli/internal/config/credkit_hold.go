// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package config

// Hand-written (handfixes.json credkit-wiring): the config and credential files
// a run may name but does not read, held at startup through the credential
// kit's own body reader, so a value in one cannot print and its base_url
// cannot become an allowed origin.

import (
	"errors"
	"os"
	"path/filepath"

	"runzero-pp-cli/internal/cliutil"
)

// HoldFile holds every credential in the config, credential store or run
// profile store at path: the kit's HoldBody reads it (every declared field at any
// depth, every duplicate; a file that does not parse has every string held),
// and the secrets of its base_url are held as the kit holds a base URL's. A
// missing or irregular file holds nothing.
func HoldFile(path string) {
	data, err := ReadRegularFile(path)
	if err != nil {
		return
	}
	cliutil.HoldBody(data)
	cliutil.HoldURL("userinfo", baseURLFromConfigBytes(data))
}

// ReadRegularFile reads path only when it is a regular file, so a FIFO or a
// device named on the command line cannot stall startup.
func ReadRegularFile(path string) ([]byte, error) {
	path = filepath.Clean(path)
	fi, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !fi.Mode().IsRegular() {
		return nil, errors.New("not a regular file")
	}
	return os.ReadFile(path) // #nosec G304 -- a config path this run may use, read only to hold its credentials
}
