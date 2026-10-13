// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"os"
	"path/filepath"
	"testing"
)

// TestApiKeyConfigured_Env covers the env-var detection path.
// Auto-refresh's API-sync branch is gated on this returning true,
// so a regression here silently disables api auto-refresh for env-auth users.
func TestApiKeyConfigured_Env(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // no config file present
	t.Setenv("GRANOLA_API_KEY", "test-key-123")
	flags := &rootFlags{}
	if !apiKeyConfigured(flags) {
		t.Fatal("expected apiKeyConfigured() == true with GRANOLA_API_KEY set")
	}
}

// TestApiKeyConfigured_Empty covers the no-auth case. Auto-refresh
// silently no-ops the api branch when this returns false; if the helper
// ever returned true for an empty environment, every command would try
// (and fail) to hit the public API.
func TestApiKeyConfigured_Empty(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("GRANOLA_API_KEY", "")
	flags := &rootFlags{}
	if apiKeyConfigured(flags) {
		t.Fatal("expected apiKeyConfigured() == false with no auth source")
	}
}

// TestApiKeyConfigured_ConfigFile covers the saved-token branch — a user
// who ran `auth set-token` has the key in their config.toml rather than
// the environment. Without this case, auto-refresh would behave correctly
// for env-auth users but silently skip the api refresh for config-auth users.
func TestApiKeyConfigured_ConfigFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GRANOLA_API_KEY", "")
	cfgDir := filepath.Join(home, ".config", "granola-cli")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("mkdir cfgDir: %v", err)
	}
	cfgPath := filepath.Join(cfgDir, "config.toml")
	if err := os.WriteFile(cfgPath, []byte("api_key = \"saved-key\"\n"), 0o600); err != nil {
		t.Fatalf("write cfg: %v", err)
	}
	flags := &rootFlags{}
	if !apiKeyConfigured(flags) {
		t.Fatal("expected apiKeyConfigured() == true with api_key in config file")
	}
}

// TestApiSyncResult_TotalRows covers the headline-count helper used by the
// provenance line. On Granola v7.4x+ the public REST API is the only refresh
// surface, so this is the sole result type the dispatcher feeds.
func TestApiSyncResult_TotalRows(t *testing.T) {
	r := ApiSyncResult{TotalRecords: 42}
	if got := r.TotalRows(); got != 42 {
		t.Errorf("TotalRows() = %d, want 42", got)
	}
}
