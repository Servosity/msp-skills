// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package config

import (
	"os"
	"path/filepath"
	"testing"
)

// Issue #132: the printing-press 4.24.0 reprint (superops-v0.1.2) dropped the
// Subdomain + Region support that v0.1.0 carried, so the CLI stopped sending
// the CustomerSubDomain header. SuperOps' nginx ingress routes by that header
// and rejects header-less requests with HTTP 400 (empty body) before they
// reach the GraphQL app - breaking every request. These tests pin the restored
// behavior so a future reprint can't silently re-drop it.
//
// Reported by @AvlCompCo (#132).

// clearSuperopsEnv neutralizes every env var Load() consults so each test sees
// a clean baseline regardless of the developer's / CI's ambient environment.
// t.Setenv to "" reads back as unset for the `!= ""` guards in Load(), and
// auto-restores the prior value when the test ends.
func clearSuperopsEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"SUPEROPS_SUBDOMAIN", "SUPEROPS_REGION", "SUPEROPS_BASE_URL",
		"SUPEROPS_API_TOKEN", "SUPEROPS_CONFIG",
	} {
		t.Setenv(k, "")
	}
}

func TestLoad_SubdomainEnvSetsCustomerSubDomainHeader(t *testing.T) {
	clearSuperopsEnv(t)
	t.Setenv("SUPEROPS_SUBDOMAIN", "acme")

	cfg, err := Load(filepath.Join(t.TempDir(), "absent.toml"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := cfg.Headers["CustomerSubDomain"]; got != "acme" {
		t.Fatalf("CustomerSubDomain header = %q, want %q", got, "acme")
	}
}

func TestLoad_SubdomainFromConfigFileSetsHeader(t *testing.T) {
	clearSuperopsEnv(t)
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("subdomain = \"acme-file\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := cfg.Headers["CustomerSubDomain"]; got != "acme-file" {
		t.Fatalf("CustomerSubDomain header = %q, want %q", got, "acme-file")
	}
}

func TestLoad_NoSubdomainOmitsHeader(t *testing.T) {
	clearSuperopsEnv(t)
	cfg, err := Load(filepath.Join(t.TempDir(), "absent.toml"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if v, ok := cfg.Headers["CustomerSubDomain"]; ok {
		t.Fatalf("CustomerSubDomain header should be absent when no subdomain set, got %q", v)
	}
}

func TestLoad_RegionEUSwitchesBaseURL(t *testing.T) {
	clearSuperopsEnv(t)
	t.Setenv("SUPEROPS_REGION", "eu")

	cfg, err := Load(filepath.Join(t.TempDir(), "absent.toml"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.BaseURL != "https://euapi.superops.ai" {
		t.Fatalf("BaseURL = %q, want EU host", cfg.BaseURL)
	}
}

func TestLoad_RegionUSKeepsDefaultBaseURL(t *testing.T) {
	clearSuperopsEnv(t)
	t.Setenv("SUPEROPS_REGION", "us")

	cfg, err := Load(filepath.Join(t.TempDir(), "absent.toml"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.BaseURL != "https://api.superops.ai" {
		t.Fatalf("BaseURL = %q, want US default", cfg.BaseURL)
	}
}

// An explicit SUPEROPS_BASE_URL (used by printing-press verify to point at a
// mock server) must still win over EU region selection.
func TestLoad_BaseURLOverrideWinsOverRegion(t *testing.T) {
	clearSuperopsEnv(t)
	t.Setenv("SUPEROPS_REGION", "eu")
	t.Setenv("SUPEROPS_BASE_URL", "http://127.0.0.1:8080")

	cfg, err := Load(filepath.Join(t.TempDir(), "absent.toml"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.BaseURL != "http://127.0.0.1:8080" {
		t.Fatalf("BaseURL = %q, want explicit override", cfg.BaseURL)
	}
}

// Config-file values (not just env vars) must be normalized: a region with
// stray case/whitespace must still route, and a subdomain with surrounding
// whitespace must not send a malformed CustomerSubDomain header (which would
// re-trigger the issue #132 HTTP 400).
func TestLoad_ConfigFileRegionAndSubdomainAreNormalized(t *testing.T) {
	clearSuperopsEnv(t)
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("region = \" EU \"\nsubdomain = \" acme \"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.BaseURL != "https://euapi.superops.ai" {
		t.Fatalf("BaseURL = %q, want EU host (region with case/whitespace must normalize)", cfg.BaseURL)
	}
	if got := cfg.Headers["CustomerSubDomain"]; got != "acme" {
		t.Fatalf("CustomerSubDomain header = %q, want trimmed %q", got, "acme")
	}
}
