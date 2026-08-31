// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package config

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Issue #266 shape, found by sweeping the fleet for it. ConnectWise Manage
// authenticates with a composite HTTP Basic credential (CW_COMPANY_ID +
// CW_PUBLIC_KEY : CW_PRIVATE_KEY) plus a CW_CLIENT_ID header. Every one of
// those is a persisted toml field, this connector predates the env-override
// machinery the rest of the fleet carries, and Load() composes the Basic
// header into Headers, which is persisted too. So both save() callers --
// `auth set-token` and, worse, `auth logout` -- wrote the environment's
// private key to ~/.config/connectwise-manage-cli/config.toml in cleartext
// (and a second time, base64-encoded, inside [headers]).

func envLoad(t *testing.T, path string) *Config {
	t.Helper()
	t.Setenv("CONNECTWISE_MANAGE_CONFIG", path)
	t.Setenv("CW_CLIENT_ID", "env-client-id")
	t.Setenv("CW_COMPANY_ID", "env-company")
	t.Setenv("CW_PUBLIC_KEY", "env-public-key")
	t.Setenv("CW_PRIVATE_KEY", "env-private-key")
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return cfg
}

func assertNoEnvSecrets(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	got := string(raw)
	for _, secret := range []string{"env-client-id", "env-company", "env-public-key", "env-private-key"} {
		if strings.Contains(got, secret) {
			t.Errorf("env-supplied %q was written to disk:\n%s", secret, got)
		}
	}
	// The composed Basic header carries the same private key, base64-encoded,
	// so a plain substring scan of the file would miss it.
	b64 := base64.StdEncoding.EncodeToString([]byte("env-company+env-public-key:env-private-key"))
	if strings.Contains(got, b64) {
		t.Errorf("the composed Basic credential (base64) was written to disk:\n%s", got)
	}
	return got
}

// `auth logout` is the worst case: it writes the file even when none existed,
// so logging out CREATES a plaintext credential file.
func TestLogoutDoesNotPersistEnvCredentials(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	cfg := envLoad(t, path)

	if err := cfg.ClearTokens(); err != nil {
		t.Fatalf("ClearTokens: %v", err)
	}
	assertNoEnvSecrets(t, path)
}

// `auth set-token` is the other save() caller.
func TestSetTokenDoesNotPersistEnvCredentials(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	cfg := envLoad(t, path)

	if err := cfg.SaveCredential("operator-pasted-token"); err != nil {
		t.Fatalf("SaveCredential: %v", err)
	}
	got := assertNoEnvSecrets(t, path)
	// The other direction: the token the operator deliberately handed the CLI
	// must still be stored, or the guard has broken set-token.
	if !strings.Contains(got, "operator-pasted-token") {
		t.Errorf("the explicitly supplied token was not persisted:\n%s", got)
	}
}

// The other direction at the file level: credentials that came from the config
// FILE (no env vars in play) must still round-trip through a save untouched.
func TestFileCredentialsSurviveASave(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	seed := "company_id = \"disk-company\"\npublic_key = \"disk-public\"\nprivate_key = \"disk-private\"\n"
	if err := os.WriteFile(path, []byte(seed), 0o600); err != nil {
		t.Fatalf("seeding config: %v", err)
	}
	t.Setenv("CONNECTWISE_MANAGE_CONFIG", path)
	os.Unsetenv("CW_CLIENT_ID")
	os.Unsetenv("CW_COMPANY_ID")
	os.Unsetenv("CW_PUBLIC_KEY")
	os.Unsetenv("CW_PRIVATE_KEY")
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if err := cfg.SaveCredential("operator-pasted-token"); err != nil {
		t.Fatalf("SaveCredential: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	got := string(raw)
	for _, want := range []string{"disk-company", "disk-public", "disk-private", "operator-pasted-token"} {
		if !strings.Contains(got, want) {
			t.Errorf("config-file value %q was dropped by a save:\n%s", want, got)
		}
	}
}
