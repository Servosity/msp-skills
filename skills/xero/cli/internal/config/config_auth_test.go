package config

import "testing"

// Xero uses composed auth: an OAuth2 Bearer access token PLUS a required
// Xero-Tenant-Id header on every call. These tests pin the hand-wired env-var
// handling so a future regeneration can't silently revert it to the generator
// default (which hardcoded an empty tenant header and read only the
// slug-derived oauth2 var).

func TestAccessTokenEnvBecomesBearer(t *testing.T) {
	t.Setenv("XERO_ACCESS_TOKEN", "tok-abc123")
	t.Setenv("XERO_ACCOUNTING_OAUTH2", "")
	t.Setenv("XERO_OAUTH2", "")
	t.Setenv("XERO_CONFIG", t.TempDir()+"/none.toml")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got, want := cfg.AuthHeader(), "Bearer tok-abc123"; got != want {
		t.Fatalf("AuthHeader() = %q, want %q", got, want)
	}
}

func TestTenantIDEnvBecomesHeader(t *testing.T) {
	t.Setenv("XERO_TENANT_ID", "11111111-2222-3333-4444-555555555555")
	t.Setenv("XERO_CONFIG", t.TempDir()+"/none.toml")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := cfg.Headers["Xero-Tenant-Id"]; got != "11111111-2222-3333-4444-555555555555" {
		t.Fatalf("Headers[Xero-Tenant-Id] = %q, want the tenant id", got)
	}
}

func TestCanonicalAccessTokenWinsOverSlugVar(t *testing.T) {
	t.Setenv("XERO_ACCESS_TOKEN", "access-tok")
	t.Setenv("XERO_ACCOUNTING_OAUTH2", "slug-tok")
	t.Setenv("XERO_OAUTH2", "legacy-tok")
	t.Setenv("XERO_CONFIG", t.TempDir()+"/none.toml")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got, want := cfg.AuthHeader(), "Bearer access-tok"; got != want {
		t.Fatalf("AuthHeader() = %q, want %q (XERO_ACCESS_TOKEN is canonical)", got, want)
	}
}

func TestLegacyOauth2EnvStillRead(t *testing.T) {
	t.Setenv("XERO_ACCESS_TOKEN", "")
	t.Setenv("XERO_ACCOUNTING_OAUTH2", "")
	t.Setenv("XERO_OAUTH2", "legacy-tok")
	t.Setenv("XERO_CONFIG", t.TempDir()+"/none.toml")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got, want := cfg.AuthHeader(), "Bearer legacy-tok"; got != want {
		t.Fatalf("AuthHeader() = %q, want %q (prior CLI's XERO_OAUTH2 must keep working)", got, want)
	}
	if got, want := cfg.AuthSource, "env:XERO_OAUTH2"; got != want {
		t.Fatalf("AuthSource = %q, want %q", got, want)
	}
}

func TestNoTenantEnvLeavesHeaderUnset(t *testing.T) {
	t.Setenv("XERO_TENANT_ID", "")
	t.Setenv("XERO_CONFIG", t.TempDir()+"/none.toml")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if _, ok := cfg.Headers["Xero-Tenant-Id"]; ok {
		t.Fatalf("Headers[Xero-Tenant-Id] should be unset when XERO_TENANT_ID is empty")
	}
}
