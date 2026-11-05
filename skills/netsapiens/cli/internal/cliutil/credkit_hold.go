// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated; handfixes.json credkit-config-and-origin-from-the-run).
// The kit reads each file Declarations.Files names once, in credkit.Start. The
// program reads a config again later (every config.Load; the MCP server on every
// tool call), and a file written after Start was never read by the kit. Every
// such read hands the bytes to the kit's own readers here, before the parse.

package cliutil

import (
	"os"
	"path/filepath"
	"strings"

	"netsapiens-pp-cli/internal/credkit"

	"github.com/pelletier/go-toml/v2"
)

// HoldBody and HoldURL are the kit's readers, variables for the reason Hold is one.
var (
	HoldBody = credkit.HoldBody
	HoldURL  = credkit.HoldURL
)

// HoldConfigFile holds a config or credentials file's credentials before the
// program parses it. HoldBody reads its declared credential fields (and every
// string of bytes that do not parse); what HoldBody does not name by key, each
// [headers] value and the password and query values inside base_url, is held
// from the parsed file.
func HoldConfigFile(data []byte) {
	HoldBody(data)
	var doc map[string]any
	if toml.Unmarshal(data, &doc) != nil {
		return
	}
	if base, ok := doc["base_url"].(string); ok {
		HoldURL("userinfo", base)
	}
	headers, _ := doc["headers"].(map[string]any)
	for name, v := range headers {
		if s, ok := v.(string); ok {
			Hold(credkit.Source("header:"+strings.ToLower(name)), s)
		}
	}
}

// HoldProfileStoreFile holds a run profile store with HoldBody: a value under a
// credential flag's name, every duplicate included, is held before anything
// prints. Only a regular file is opened.
func HoldProfileStoreFile(path string) {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return
	}
	if data, err := os.ReadFile(filepath.Clean(path)); err == nil {
		HoldBody(data)
	}
}
