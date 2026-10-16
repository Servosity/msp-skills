// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// hand-fix guard (handfixes.json env-base-url-never-persisted): an env-sourced
// base URL must not be written to config.toml, so unsetting the variable
// restores the default on the next run.
func TestEnvBaseURLNeverPersisted(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	t.Setenv("BITDEFENDER_API_KEY", "")
	t.Setenv("BITDEFENDER_BASE_URL", "http://127.0.0.1:1/wrongbase")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.BaseURL != "http://127.0.0.1:1/wrongbase" {
		t.Fatalf("env override not applied: %q", cfg.BaseURL)
	}
	if err := cfg.SaveCredential("test-secret"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "wrongbase") {
		t.Fatalf("env base URL persisted to config.toml:\n%s", data)
	}
	t.Setenv("BITDEFENDER_BASE_URL", "")
	cfg, err = Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.BaseURL != "https://cloud.gravityzone.bitdefender.com/api" {
		t.Fatalf("unset env did not restore the default base URL: %q", cfg.BaseURL)
	}
}
