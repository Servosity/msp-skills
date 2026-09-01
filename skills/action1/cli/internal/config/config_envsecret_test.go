// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Issue #266: OAuth client credentials that reached the process through the
// environment must never be copied into the on-disk config, while credentials
// the caller supplied deliberately still must be. Persistence is decided by
// declared provenance (MarkCredentialsExplicit), not by comparing values --
// the two cases can carry identical strings, and a caller that trims its env
// fallback while Load does not would make a padded variable compare unequal to
// itself. Each test below fails if that guarantee regresses.

func loadWithEnvCreds(t *testing.T, envSecret string) (*Config, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	t.Setenv("ACTION1_CONFIG", path)
	t.Setenv("ACTION1_CLIENT_ID", "env-id")
	t.Setenv("ACTION1_CLIENT_SECRET", envSecret)
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return cfg, path
}

func savedConfig(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return string(b)
}

// The reported defect: a token mint on an ordinary authenticated command wrote
// the env-supplied credentials to disk.
func TestEnvClientCredentialsAreNotPersisted(t *testing.T) {
	cfg, path := loadWithEnvCreds(t, "env-secret")
	if !cfg.envOverrides["ClientSecret"] {
		t.Fatal("precondition: Load did not mark ClientSecret as an env override")
	}
	if !cfg.envOverrides["ClientID"] {
		t.Fatal("precondition: Load did not mark ClientID as an env override")
	}

	if err := cfg.SaveTokens(cfg.ClientID, cfg.ClientSecret, "minted-token", "", time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("SaveTokens: %v", err)
	}

	got := savedConfig(t, path)
	for _, secret := range []string{"env-secret", "env-id"} {
		if strings.Contains(got, secret) {
			t.Errorf("env-supplied %q was written to disk:\n%s", secret, got)
		}
	}
	if !strings.Contains(got, "minted-token") {
		t.Errorf("the minted access token should still be cached:\n%s", got)
	}
}

// A caller that resolves its credentials from the environment and trims them
// produces a value that differs from what Load stored, so a value comparison
// would treat it as caller-supplied and persist it.
func TestTrimmedEnvCredentialsAreNotPersisted(t *testing.T) {
	cfg, path := loadWithEnvCreds(t, "  env-secret  ")

	// Nothing was declared explicit.
	cfg.MarkCredentialsExplicit(false, false)
	if err := cfg.SaveTokens(strings.TrimSpace(cfg.ClientID), strings.TrimSpace(cfg.ClientSecret), "minted-token", "", time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("SaveTokens: %v", err)
	}

	if got := savedConfig(t, path); strings.Contains(got, "env-secret") {
		t.Errorf("a whitespace-padded env secret reached disk once trimmed:\n%s", got)
	}
}

// The other direction: a credential the caller declared explicit must still be
// stored, or a connector could never persist one at all.
func TestExplicitCredentialsStillPersist(t *testing.T) {
	cfg, path := loadWithEnvCreds(t, "env-secret")

	cfg.MarkCredentialsExplicit(true, true)
	if err := cfg.SaveTokens("flag-id", "flag-secret", "minted-token", "", time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("SaveTokens: %v", err)
	}

	got := savedConfig(t, path)
	for _, want := range []string{"flag-id", "flag-secret"} {
		if !strings.Contains(got, want) {
			t.Errorf("explicit credential %q was not persisted:\n%s", want, got)
		}
	}
	if strings.Contains(got, "env-secret") {
		t.Errorf("env-supplied secret leaked alongside the explicit one:\n%s", got)
	}
}

// An explicit value that happens to equal the environment's is still explicit.
func TestExplicitCredentialEqualToEnvStillPersists(t *testing.T) {
	cfg, path := loadWithEnvCreds(t, "env-secret")

	cfg.MarkCredentialsExplicit(true, true)
	if err := cfg.SaveTokens("env-id", "env-secret", "minted-token", "", time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("SaveTokens: %v", err)
	}

	got := savedConfig(t, path)
	for _, want := range []string{"env-id", "env-secret"} {
		if !strings.Contains(got, want) {
			t.Errorf("explicitly passed credential %q was dropped:\n%s", want, got)
		}
	}
}

// Provenance is per field.
func TestPartialExplicitCredentials(t *testing.T) {
	cfg, path := loadWithEnvCreds(t, "env-secret")

	cfg.MarkCredentialsExplicit(true, false)
	if err := cfg.SaveTokens("flag-id", cfg.ClientSecret, "minted-token", "", time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("SaveTokens: %v", err)
	}

	got := savedConfig(t, path)
	if !strings.Contains(got, "flag-id") {
		t.Errorf("explicit client ID was not persisted:\n%s", got)
	}
	if strings.Contains(got, "env-secret") {
		t.Errorf("env-supplied secret was persisted:\n%s", got)
	}
}

// The `auth set-token` shape: no env credentials in play at all, so the token
// the operator deliberately handed the CLI must land on disk unchanged. This
// is the direction a too-eager guard would break.
func TestSetTokenStyleSaveStillPersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	t.Setenv("ACTION1_CONFIG", path)
	os.Unsetenv("ACTION1_CLIENT_ID")
	os.Unsetenv("ACTION1_CLIENT_SECRET")
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	cfg.AuthHeaderVal = ""
	if err := cfg.SaveTokens("", "", "operator-pasted-token", "", cfg.TokenExpiry); err != nil {
		t.Fatalf("SaveTokens: %v", err)
	}
	if got := savedConfig(t, path); !strings.Contains(got, "operator-pasted-token") {
		t.Errorf("an explicit set-token save did not persist:\n%s", got)
	}
}

// `auth logout` must still erase a client_id/client_secret that WAS persisted
// on disk, even while the env vars are exported: the wipe is not a credential
// write and must not be suppressed by the env-override machinery.
func TestLogoutStillErasesPersistedCredentials(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("client_id = \"disk-id\"\nclient_secret = \"disk-secret\"\naccess_token = \"disk-token\"\n"), 0o600); err != nil {
		t.Fatalf("seeding config: %v", err)
	}
	t.Setenv("ACTION1_CONFIG", path)
	t.Setenv("ACTION1_CLIENT_ID", "env-id")
	t.Setenv("ACTION1_CLIENT_SECRET", "env-secret")
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if err := cfg.ClearTokens(); err != nil {
		t.Fatalf("ClearTokens: %v", err)
	}

	got := savedConfig(t, path)
	for _, gone := range []string{"disk-id", "disk-secret", "disk-token", "env-id", "env-secret"} {
		if strings.Contains(got, gone) {
			t.Errorf("logout left %q on disk:\n%s", gone, got)
		}
	}
}
