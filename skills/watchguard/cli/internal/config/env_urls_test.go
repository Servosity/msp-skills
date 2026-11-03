// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// hand-fix guard (handfixes.json env-urls-never-persisted): an env-sourced
// region URL or account ID must not be written to config.toml, so unsetting
// the variable restores the default on the next run.
func TestEnvURLsNeverPersisted(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	t.Setenv("WATCHGUARD_BASE_URL", "http://127.0.0.1:1/wrongbase")
	t.Setenv("WATCHGUARD_TOKEN_URL", "http://127.0.0.1:1/wrongtoken")
	t.Setenv("WATCHGUARD_ACCOUNT_ID", "ACC-wrongacct")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.BaseURL != "http://127.0.0.1:1/wrongbase" || cfg.TemplateVars["accountId"] != "ACC-wrongacct" {
		t.Fatalf("env override not applied: %q %v", cfg.BaseURL, cfg.TemplateVars)
	}
	if err := cfg.save(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "wrong") {
		t.Fatalf("env override persisted to config.toml:\n%s", data)
	}
	t.Setenv("WATCHGUARD_BASE_URL", "")
	t.Setenv("WATCHGUARD_TOKEN_URL", "")
	t.Setenv("WATCHGUARD_ACCOUNT_ID", "")
	cfg, err = Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.BaseURL != "https://api.usa.cloud.watchguard.com" {
		t.Fatalf("unset env did not restore the default base URL: %q", cfg.BaseURL)
	}
	if cfg.TemplateVars["accountId"] != "" {
		t.Fatalf("unset env left a stale account ID: %q", cfg.TemplateVars["accountId"])
	}
}
