// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): credkit-wiring response cache.

package client

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"

	"superops-pp-cli/internal/credkit"
)

// A response body that repeats a held credential, in any encoding, is never
// written to the response cache; a clean body still is.
func TestResponseCacheNeverKeepsAHeldCredential(t *testing.T) {
	if !heldChild(t) {
		return
	}
	const held = "cacheHeldTok7713"
	credkit.Hold("minted:test", held)
	c := &Client{BaseURL: "https://api.example.test", cacheDir: t.TempDir()}
	c.writeCache("/v1/leak", nil, json.RawMessage(`{"note":"`+hex.EncodeToString([]byte(held))+`"}`))
	if _, err := os.Stat(c.cacheResourceDir("/v1/leak")); err == nil {
		entries, _ := os.ReadDir(c.cacheResourceDir("/v1/leak"))
		if len(entries) > 0 {
			t.Fatalf("a body repeating a held credential was cached: %v", entries)
		}
	}
	c.writeCache("/v1/clean", nil, json.RawMessage(`{"note":"fine"}`))
	if entries, _ := os.ReadDir(c.cacheResourceDir("/v1/clean")); len(entries) == 0 {
		t.Fatal("a clean body was not cached")
	}
}
