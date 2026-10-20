// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"eset-protect-pp-cli/internal/secrets"
)

// TestConfigFileCredentialsAreRegistered (handfixes.json:
// secret-registry-masks-every-boundary): a credential stored in the config
// file is a secret from the moment Load reads it, and so is a custom header
// whose name marks it as a credential; an ordinary header value is not.
func TestConfigFileCredentialsAreRegistered(t *testing.T) {
	isolateCredentials(t)
	path := filepath.Join(t.TempDir(), "config.toml")
	body := "protect_password = \"file-password-1A3S5D\"\n[headers]\nX-Api-Key = \"header-key-7F9G1H\"\nAccept = \"application/vnd.eset+json\"\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err != nil {
		t.Fatal(err)
	}
	got := secrets.Mask("file-password-1A3S5D header-key-7F9G1H application/vnd.eset+json")
	if strings.Contains(got, "1A3S5D") || strings.Contains(got, "7F9G1H") {
		t.Fatalf("a config-file credential was not registered on Load: %q", got)
	}
	if !strings.Contains(got, "application/vnd.eset+json") {
		t.Fatalf("an ordinary header value was masked: %q", got)
	}
}

// TestSavedTokensAreRegistered: a token handed to SaveTokens (auth
// set-token, a mint) is a secret from that moment.
func TestSavedTokensAreRegistered(t *testing.T) {
	isolateCredentials(t)
	cfg, err := Load(filepath.Join(t.TempDir(), "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	_ = cfg.SaveTokens("", "", "saved-access-3J5K7L", "saved-refresh-9Z1X3C", time.Now().Add(time.Hour))
	if got := secrets.Mask("saved-access-3J5K7L saved-refresh-9Z1X3C"); strings.Contains(got, "3J5K7L") || strings.Contains(got, "9Z1X3C") {
		t.Fatalf("a saved token was not registered: %q", got)
	}
}
