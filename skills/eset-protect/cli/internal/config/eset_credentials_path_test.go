// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"eset-protect-pp-cli/internal/cliutil"
)

// TestExplicitConfigLogoutClearsItsOwnCredentials (handfixes.json:
// credentials-path-follows-config): Load reads an explicit --config's
// colocated data/credentials.toml, but ClearTokens and SaveTokens used the
// global file, so `--config tenantA/config.toml auth logout` left tenant A
// authenticated and deleted the default configuration's credentials.
func TestExplicitConfigLogoutClearsItsOwnCredentials(t *testing.T) {
	isolateCredentials(t)
	if err := cliutil.SaveCredentials(&cliutil.Credentials{AccessToken: "token-global"}); err != nil {
		t.Fatal(err)
	}
	globalPath, err := cliutil.CredentialsFilePath()
	if err != nil {
		t.Fatal(err)
	}
	tenant := filepath.Join(t.TempDir(), "tenantA")
	if err := os.MkdirAll(tenant, 0o700); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(tenant, "config.toml")
	if err := os.WriteFile(configPath, []byte(""), 0o600); err != nil {
		t.Fatal(err)
	}
	colocated, err := cliutil.CredentialsFilePathForConfig(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := cliutil.SaveCredentialsAt(colocated, &cliutil.Credentials{AccessToken: "token-a"}); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AccessToken != "token-a" {
		t.Fatalf("tenant A loaded %q, want its colocated token", cfg.AccessToken)
	}
	if err := cfg.SaveTokens("", "", "token-a2", "", time.Time{}); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(colocated); !strings.Contains(string(got), "token-a2") {
		t.Fatalf("SaveTokens did not write tenant A's colocated file: %q", got)
	}
	if got, _ := os.ReadFile(globalPath); strings.Contains(string(got), "token-a2") {
		t.Fatal("SaveTokens under tenant A overwrote the default credentials")
	}
	if err := cfg.ClearTokens(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(colocated); !os.IsNotExist(err) {
		t.Fatalf("logout left tenant A's credentials file: %v", err)
	}
	if _, err := os.Stat(globalPath); err != nil {
		t.Fatalf("logout under tenant A deleted the default credentials: %v", err)
	}

	// With its own file gone, tenant A falls back to the global file, as
	// Load always has.
	cfg, err = Load(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AccessToken != "token-global" {
		t.Fatalf("after logout tenant A should fall back to the global file, got %q", cfg.AccessToken)
	}
}
