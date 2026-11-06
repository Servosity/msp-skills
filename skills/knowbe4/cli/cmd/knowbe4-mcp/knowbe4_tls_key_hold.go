// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated; handfixes.json credkit-wiring).

package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"knowbe4-pp-cli/internal/credkit"
)

// holdTLSKey holds the private key the MCP HTTP server would load from path:
// each base64 line of its PEM body, and the body joined. It is bound nowhere
// (credwire: minted:tls-key), so no output and no request can carry it.
func holdTLSKey(path string) {
	if strings.TrimSpace(path) == "" {
		return
	}
	data, err := fs.ReadFile(os.DirFS(filepath.Dir(path)), filepath.Base(path))
	if err != nil {
		return
	}
	var body []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "-----") {
			body = append(body, line)
		}
	}
	credkit.Hold("minted:tls-key", append(body, strings.Join(body, ""))...)
}
