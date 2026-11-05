// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): handfixes.json credkit-wiring.

package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"quickbooks-pp-cli/internal/credkit"
)

// qboHoldTLSKey holds the --tls-key file's key material with the credential
// kit: each line inside the PEM armor and the body as one string. An unreadable
// file holds nothing; the listener then fails on the same file.
func qboHoldTLSKey(path string) {
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
