// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored regression suite for token_lifetime.
//
// internal/client clamps its refresh safety skew to half the lifetime the token
// endpoint granted. Without that clamp a token endpoint issuing short-lived
// tokens puts every freshly minted token inside the 5-minute skew the instant it
// arrives, so the client re-mints on EVERY request behind a package-level mutex
// and serializes the whole fan-out. The clamp can only work if the lifetime
// survives the round trip to disk.

package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTokenLifetime_RoundTripsThroughDisk(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	cfg := &Config{Path: path}
	cfg.SetOAuthContext("tenant-abc", "", "")
	cfg.SetTokenLifetime(120)
	if err := cfg.SaveTokens("cid", "csec", "tok", "", time.Now().Add(120*time.Second)); err != nil {
		t.Fatalf("SaveTokens: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if !strings.Contains(string(data), "token_lifetime") {
		t.Fatalf("token_lifetime not written to disk:\n%s", data)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.TokenLifetime != 120 {
		t.Fatalf("TokenLifetime = %d, want 120", got.TokenLifetime)
	}
}

// TestTokenLifetime_AbsentMeansUnknown pins the migration direction: every
// config written before this field existed loads as 0, which internal/client
// reads as "unknown" and answers with the unchanged 5-minute default skew.
func TestTokenLifetime_AbsentMeansUnknown(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	legacy := "base_url = \"https://cipp.example.com/api\"\naccess_token = \"tok\"\nclient_id = \"cid\"\nclient_secret = \"csec\"\n"
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.TokenLifetime != 0 {
		t.Fatalf("a pre-migration config must report an unknown lifetime, got %d", got.TokenLifetime)
	}
}

// TestTokenLifetime_ClearedByLogout: a lifetime left behind by logout would
// mis-clamp the skew for whatever credential is configured next.
func TestTokenLifetime_ClearedByLogout(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	cfg := &Config{Path: path}
	cfg.SetTokenLifetime(120)
	if err := cfg.SaveTokens("cid", "csec", "tok", "", time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("SaveTokens: %v", err)
	}
	if err := cfg.ClearTokens(); err != nil {
		t.Fatalf("ClearTokens: %v", err)
	}
	if cfg.TokenLifetime != 0 {
		t.Fatalf("in-memory TokenLifetime = %d after logout, want 0", cfg.TokenLifetime)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.TokenLifetime != 0 {
		t.Fatalf("on-disk token_lifetime = %d after logout, want 0", got.TokenLifetime)
	}
}

// TestSetTokenLifetime_RejectsNegative: a negative expires_in is nonsense; it
// must not become a negative skew, which would disable refresh entirely.
func TestSetTokenLifetime_RejectsNegative(t *testing.T) {
	cfg := &Config{Path: filepath.Join(t.TempDir(), "config.toml")}
	cfg.SetTokenLifetime(-5)
	if cfg.TokenLifetime != 0 {
		t.Fatalf("TokenLifetime = %d, want 0", cfg.TokenLifetime)
	}
}
