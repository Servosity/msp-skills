// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written (NOT generated): regression guards for handfixes.json entry
// "credkit-wiring" (class secret-echo).

package client

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"avanan-pp-cli/internal/credkit"
)

// TestResponseCacheNeverStoresAHeldValue: a body carrying a held credential is
// not cached (the kit refuses the write).
func TestResponseCacheNeverStoresAHeldValue(t *testing.T) {
	const v = "cache-held-value-CH1"
	c := &Client{BaseURL: "https://api.example.test", cacheDir: t.TempDir()}
	credkit.Hold("config:unclassified", v)
	c.writeCacheWithHeaders("/v1.0/late", nil, nil, json.RawMessage(`{"k":"`+v+`"}`))
	_ = filepath.WalkDir(c.cacheDir, func(path string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if b, _ := os.ReadFile(path); strings.Contains(string(b), v) {
				t.Errorf("%s carries the held value", path)
			}
		}
		return nil
	})
}
