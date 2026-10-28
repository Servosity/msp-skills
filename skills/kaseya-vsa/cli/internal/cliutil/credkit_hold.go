// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated; handfixes.json credkit-wiring).
// A config or credentials file read after the kit's startup read (the MCP
// server reloads the file on every tool call, and a store written since) is
// held with the kit's own readers before it is parsed: its credential fields,
// every word of a file that does not parse, and the password and query values
// of its base_url.

package cliutil

import (
	"kaseya-vsa-pp-cli/internal/credkit"

	"github.com/pelletier/go-toml/v2"
)

// HoldConfigFile holds the credentials in a config or credentials file.
func HoldConfigFile(data []byte) {
	credkit.HoldBody(data)
	var parsed struct {
		BaseURL string `toml:"base_url"`
	}
	if toml.Unmarshal(data, &parsed) == nil {
		credkit.HoldURL("userinfo", parsed.BaseURL)
	}
}
