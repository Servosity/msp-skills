// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package config

// Hand-written (handfixes.json env-clientid-never-saved).

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestEnvClientIDNeverSaved (kit note c): config.Load injects
// CONNECTWISE_AUTOMATE_CLIENT_ID into Headers, and every save wrote Headers to
// config.toml, so `auth set-token` persisted a credential the operator kept in
// the environment. A save keeps the config file's own clientId (or none).
func TestEnvClientIDNeverSaved(t *testing.T) {
	const envID = "cwaEnvClient" + "-7c1e9a3f5b"
	const fileID = "cwaFileClient" + "-2d8b6e0a4c"
	for _, tc := range []struct {
		name, file, keep string
	}{
		{"no clientId in the file", "base_url = \"https://acme.example/cwa/api/v1\"\n", ""},
		{"the file's own clientId", "base_url = \"https://acme.example/cwa/api/v1\"\n\n[headers]\nclientId = \"" + fileID + "\"\nX-Extra = \"kept-header\"\n", fileID},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)
			t.Setenv("CONNECTWISE_AUTOMATE_HOME", home)
			t.Setenv("CONNECTWISE_AUTOMATE_CLIENT_ID", envID)
			dir := filepath.Join(home, "profile")
			if err := os.MkdirAll(dir, 0o700); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, "config.toml")
			if err := os.WriteFile(path, []byte(tc.file), 0o600); err != nil {
				t.Fatal(err)
			}
			cfg, err := Load(path)
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if cfg.Headers["clientId"] != envID {
				t.Fatalf("clientId header = %q, want the environment's", cfg.Headers["clientId"])
			}
			if err := cfg.SaveTokens("", "", "cwaSaved"+"Tok-91e3c5a7", "", cfg.TokenExpiry); err != nil {
				t.Fatalf("SaveTokens: %v", err)
			}
			_ = filepath.WalkDir(home, func(p string, d fs.DirEntry, err error) error {
				if err != nil || d.IsDir() {
					return nil
				}
				if data, _ := os.ReadFile(p); bytes.Contains(data, []byte(envID)) {
					t.Errorf("%s holds the environment's clientId:\n%s", p, data)
				}
				return nil
			})
			saved, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if tc.keep != "" && !strings.Contains(string(saved), tc.keep) {
				t.Errorf("the save dropped the file's own clientId:\n%s", saved)
			}
			if tc.keep != "" && !strings.Contains(string(saved), "kept-header") {
				t.Errorf("the save dropped another header:\n%s", saved)
			}
		})
	}
}
