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
// the caller typed on `auth login` still must be. Persistence is decided by
// declared provenance (MarkCredentialsExplicit), not by comparing values --
// the two cases can carry identical strings, and `auth login` trims its env
// fallback while Load does not, so a padded variable would compare unequal to
// itself. Each test below fails if that guarantee regresses.

func loadWithEnvCreds(t *testing.T, envSecret string) (*Config, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	t.Setenv("PAX8_CONFIG", path)
	t.Setenv("PAX8_CLIENT_ID", "env-id")
	t.Setenv("PAX8_CLIENT_SECRET", envSecret)
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

// `auth login` with no flags resolves the credentials from the environment and
// trims them. The trimmed value differs from what Load stored, so a value
// comparison would treat it as caller-supplied and persist it.
func TestTrimmedEnvCredentialsAreNotPersisted(t *testing.T) {
	cfg, path := loadWithEnvCreds(t, "  env-secret  ")

	// No flags were given, so nothing is declared explicit.
	cfg.MarkCredentialsExplicit(false, false)
	if err := cfg.SaveTokens(strings.TrimSpace(cfg.ClientID), strings.TrimSpace(cfg.ClientSecret), "minted-token", "", time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("SaveTokens: %v", err)
	}

	if got := savedConfig(t, path); strings.Contains(got, "env-secret") {
		t.Errorf("a whitespace-padded env secret reached disk once trimmed:\n%s", got)
	}
}

// The other direction: an explicit `auth login --client-id/--client-secret`
// must still be stored.
func TestExplicitLoginCredentialsStillPersist(t *testing.T) {
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

// An explicit flag whose value happens to equal the environment's is still
// explicit -- `auth login --client-secret "$SECRET"` must persist, or the CLI
// cannot re-mint once the variable is gone.
func TestExplicitFlagEqualToEnvStillPersists(t *testing.T) {
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

// Provenance is per field: --client-id alone stores the ID and leaves the
// env-supplied secret off disk.
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
