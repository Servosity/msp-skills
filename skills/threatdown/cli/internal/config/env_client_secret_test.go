// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package config

import (
	"path/filepath"
	"testing"
	"time"
)

// hand-fix guard (handfixes.json token-bound-to-client): a client ID from the
// environment that differs from the saved client never borrows the secret
// saved for that client, and loading it does not wipe the saved secret.
func TestEnvClientIDNeverPairsWithSavedSecret(t *testing.T) {
	home := t.TempDir()
	t.Setenv("THREATDOWN_HOME", home)
	t.Setenv("THREATDOWN_CLIENT_ID", "")
	t.Setenv("THREATDOWN_CLIENT_SECRET", "")
	path := filepath.Join(home, "config.toml")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.SaveTokens("admin", "adminsec", "tok", "", time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	t.Setenv("THREATDOWN_CLIENT_ID", "reporting")
	cfg, err = Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ClientSecret != "" {
		t.Fatalf("env client ID paired with the saved secret %q", cfg.ClientSecret)
	}
	if err := cfg.saveCredentialsThenConfig(); err != nil {
		t.Fatal(err)
	}
	t.Setenv("THREATDOWN_CLIENT_ID", "admin")
	cfg, err = Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ClientSecret != "adminsec" {
		t.Fatalf("saved secret lost: %q", cfg.ClientSecret)
	}
}
