// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package config

import (
	"encoding/base32"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"knowbe4-pp-cli/internal/cliutil/testenv"
)

// Class persistence-gate-narrower-than-chokepoint (Codex r1): config.toml's
// header check used the shared funnel's narrower matcher and the last check
// skipped the header values, so a header carrying the API key in base32 was
// saved. Every byte written is now judged by the whole chokepoint.
func TestR1ConfigHeaderEncodingOfTheKeyIsNotSaved(t *testing.T) {
	testenv.Isolate(t)
	const heldVal = "CompressedHeldSecret-7a3f9c1e5b"
	for name, v := range map[string]string{
		"base32":          base32.StdEncoding.EncodeToString([]byte(heldVal)),
		"base32 raw":      strings.TrimRight(base32.StdEncoding.EncodeToString([]byte(heldVal)), "="),
		"base64 url":      base64.RawURLEncoding.EncodeToString([]byte(heldVal)),
		"spaced":          "Compressed HeldSecret-7a3f9c1e5b",
		"reversed halves": "-7a3f9c1e5b CompressedHeldSecret",
	} {
		dir := t.TempDir()
		path := filepath.Join(dir, "config.toml")
		if err := os.WriteFile(path, []byte("[headers]\nX-Trace = \""+v+"\"\nX-Keep = \"keep-me\"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		t.Setenv("KNOWBE4_API_KEY", heldVal)
		cfg, err := Load(path)
		if err != nil {
			t.Fatal(err)
		}
		_ = cfg.SaveTokens("", "", heldVal, "", cfg.TokenExpiry) // auth set-token
		got, _ := os.ReadFile(path)
		if strings.Contains(string(got), v) {
			t.Errorf("%s: config.toml keeps the API key's %s form:\n%s", name, name, got)
		}
		if !strings.Contains(string(got), "keep-me") {
			t.Errorf("%s: config.toml lost the unrelated header X-Keep:\n%s", name, got)
		}
	}
}
