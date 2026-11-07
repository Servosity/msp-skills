// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated; handfixes.json credkit-wiring).
// The connector reads no document of its own: a config, credential or profile
// file goes to credkit.HoldBody, a URL to credkit.HoldURL. What stays here is
// what HoldBody does not read from a config: its [headers] values and the
// credentials inside its base_url.

package cliutil

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"salesbuildr-pp-cli/internal/credkit"

	"github.com/pelletier/go-toml/v2"
)

// Hold, HoldBody and HoldURL are the kit's. Production code never reassigns them.
// In-process unit tests may, to no-ops: a held value lasts for the process and the
// kit withholds whatever carries it, so one test's fixture would withhold the next
// test's output. The built binaries' leak tests (internal/credwire) prove the real thing.
var (
	Hold     = credkit.Hold
	HoldBody = credkit.HoldBody
	HoldURL  = credkit.HoldURL
)

// HoldConfigFile holds a config, credential or profile file's credentials before
// it is parsed, with HoldBody. Kept beside it: a [headers] value and the
// password or a query value inside base_url, which contract 1 names and HoldBody
// does not read from a config.
func HoldConfigFile(data []byte) {
	HoldBody(data)
	var parsed struct {
		BaseURL string         `toml:"base_url"`
		Headers map[string]any `toml:"headers"`
	}
	if toml.Unmarshal(data, &parsed) != nil {
		return // bytes that do not parse: HoldBody held every string in them
	}
	HoldURL("userinfo", parsed.BaseURL) // not config:base_url, which an Identifier exempts
	for name, v := range parsed.Headers {
		switch v.(type) {
		case string, int64, float64:
			Hold("header:"+strings.ToLower(name), fmt.Sprint(v))
		}
	}
}

// HoldFile is HoldConfigFile on a file. Only a regular file is opened, so a FIFO
// named on the command line cannot hang startup.
func HoldFile(path string) {
	if data, err := readRegularFile(path); err == nil {
		HoldConfigFile(data)
	}
}

func readRegularFile(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular file", path)
	}
	return os.ReadFile(filepath.Clean(path)) // the operator's own config, store or profile path, read to hold its values
}
