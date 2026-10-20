// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// hand-fix guard (handfixes.json env-base-url-never-persisted): an env-sourced
// ESET_PROTECT_BASE_URL must not be written to config.toml when the CLI saves
// a minted token. A persisted override would keep steering requests (and,
// through the credential host allowlist, the bearer token) to that host after
// the variable is unset.
func TestEnvBaseURLNeverPersisted(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	for _, k := range []string{"ESET_PROTECT_USERNAME", "ESET_PROTECT_PASSWORD", "ESET_PROTECT_TOKEN", "ESET_PROTECT_REGION", "ESET_PROTECT_CONFIG"} {
		t.Setenv(k, "")
	}
	t.Setenv("ESET_PROTECT_BASE_URL", "http://127.0.0.1:1/wrongbase")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.BaseURL != "http://127.0.0.1:1/wrongbase" {
		t.Fatalf("env override not applied: %q", cfg.BaseURL)
	}
	if err := cfg.SaveTokens("eset:eu:0000000000000000", "", "test-access", "test-refresh", time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "wrongbase") {
		t.Fatalf("env base URL persisted to config.toml:\n%s", data)
	}
	t.Setenv("ESET_PROTECT_BASE_URL", "")
	cfg, err = Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.BaseURL != "https://{region}.device-management.eset.systems" {
		t.Fatalf("unset env did not restore the default base URL: %q", cfg.BaseURL)
	}
	if cfg.AccessToken != "test-access" {
		t.Fatalf("saved token lost: %q", cfg.AccessToken)
	}
}
