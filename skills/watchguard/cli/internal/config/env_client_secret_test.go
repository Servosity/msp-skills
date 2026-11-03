// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// hand-fix guard (handfixes.json token-bound-to-client): a client ID from the
// environment that differs from the saved client never borrows the secret
// saved for that client, and loading it does not wipe the saved secret.
func TestEnvClientIDNeverPairsWithSavedSecret(t *testing.T) {
	home := t.TempDir()
	t.Setenv("WATCHGUARD_HOME", home)
	t.Setenv("WATCHGUARD_CONFIG", "")
	t.Setenv("WATCHGUARD_CLIENT_ID", "")
	t.Setenv("WATCHGUARD_CLIENT_SECRET", "")
	t.Setenv("WATCHGUARD_API_KEY", "")
	path := filepath.Join(home, "config.toml")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.SaveTokens("rw", "rwsec", "tok", "", time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WATCHGUARD_CLIENT_ID", "ro")
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
	t.Setenv("WATCHGUARD_CLIENT_ID", "rw")
	cfg, err = Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ClientSecret != "rwsec" {
		t.Fatalf("saved secret lost: %q", cfg.ClientSecret)
	}
}

// A second token save in one process (a long sync or tail that mints twice
// under env credentials) keeps the client pair and API key a login saved.
func TestRepeatedSavesKeepSavedClientPair(t *testing.T) {
	home := t.TempDir()
	t.Setenv("WATCHGUARD_HOME", home)
	t.Setenv("WATCHGUARD_CONFIG", "")
	t.Setenv("WATCHGUARD_CLIENT_ID", "")
	t.Setenv("WATCHGUARD_CLIENT_SECRET", "")
	t.Setenv("WATCHGUARD_API_KEY", "")
	path := filepath.Join(home, "config.toml")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	cfg.WatchguardApiKey = "savedkey"
	if err := cfg.SaveTokens("rw", "rwsec", "tok", "", time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WATCHGUARD_CLIENT_ID", "ro")
	t.Setenv("WATCHGUARD_CLIENT_SECRET", "rosec")
	t.Setenv("WATCHGUARD_API_KEY", "envkey")
	cfg, err = Load(path)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := cfg.SaveTokens("ro", "rosec", "tok2", "", time.Now().Add(time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("WATCHGUARD_CLIENT_ID", "")
	t.Setenv("WATCHGUARD_CLIENT_SECRET", "")
	t.Setenv("WATCHGUARD_API_KEY", "")
	cfg, err = Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ClientID != "rw" || cfg.ClientSecret != "rwsec" {
		t.Fatalf("saved login lost after repeated saves: id=%q secret=%q", cfg.ClientID, cfg.ClientSecret)
	}
	if cfg.WatchguardApiKey != "savedkey" {
		t.Fatalf("saved WatchGuard-API-Key lost after repeated saves: %q", cfg.WatchguardApiKey)
	}
}

// The binding fingerprint is what SaveTokens persists and Load restores, and
// it distinguishes the client, scope and token URL.
func TestTokenBindingPersistsAndDiscriminates(t *testing.T) {
	home := t.TempDir()
	t.Setenv("WATCHGUARD_HOME", home)
	t.Setenv("WATCHGUARD_CONFIG", "")
	t.Setenv("WATCHGUARD_CLIENT_ID", "")
	t.Setenv("WATCHGUARD_CLIENT_SECRET", "")
	t.Setenv("WATCHGUARD_API_KEY", "")
	const tokenURL = "https://api.usa.cloud.watchguard.com/oauth/token"
	b := TokenBindingFor("rw", "api-access", tokenURL)
	for name, other := range map[string]string{
		"client":    TokenBindingFor("ro", "api-access", tokenURL),
		"scope":     TokenBindingFor("rw", "other", tokenURL),
		"token URL": TokenBindingFor("rw", "api-access", "https://api.deu.cloud.watchguard.com/oauth/token"),
	} {
		if other == b {
			t.Errorf("a different %s produced the same binding", name)
		}
	}
	if TokenBindingFor("rw", " api-access ", tokenURL) != b {
		t.Error("whitespace around the scope changed the binding")
	}
	path := filepath.Join(home, "config.toml")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	cfg.TokenBinding = b
	if err := cfg.SaveTokens("rw", "rwsec", "tok", "", time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	cfg, err = Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.TokenBinding != b {
		t.Fatalf("binding not persisted: %q", cfg.TokenBinding)
	}
	if err := cfg.ClearTokens(); err != nil {
		t.Fatal(err)
	}
	cfg, err = Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.TokenBinding != "" {
		t.Fatalf("logout left the binding behind: %q", cfg.TokenBinding)
	}
}

// Under agentcookie, save() writes the full config, so logout must zero the
// binding itself or config.toml keeps the old client's fingerprint.
func TestLogoutClearsBindingUnderAgentcookie(t *testing.T) {
	home := t.TempDir()
	t.Setenv("WATCHGUARD_HOME", home)
	t.Setenv("WATCHGUARD_CONFIG", "")
	t.Setenv("WATCHGUARD_CLIENT_ID", "")
	t.Setenv("WATCHGUARD_CLIENT_SECRET", "")
	t.Setenv("WATCHGUARD_API_KEY", "")
	path := filepath.Join(home, "config.toml")
	if err := os.WriteFile(filepath.Join(home, ".agentcookie-managed"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	cfg.TokenBinding = TokenBindingFor("rw", "api-access", "https://api.usa.cloud.watchguard.com/oauth/token")
	if err := cfg.SaveTokens("rw", "rwsec", "tok", "", time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := cfg.ClearTokens(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "token_binding") {
		t.Fatalf("logout left token_binding in config.toml:\n%s", data)
	}
}
