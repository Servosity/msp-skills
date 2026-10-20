// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/dnsfilter/handfixes.json
// under the "credentials-file-follows-config" entry.

package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dnsfilter-pp-cli/internal/cliutil"
	"dnsfilter-pp-cli/internal/cliutil/testenv"
)

// An explicit config's credentials live in its sibling data/credentials.toml:
// set-token writes there, a load reads it, and logout clears it without
// touching the default credentials file.
func TestExplicitConfigCredentialsFollowTheConfig(t *testing.T) {
	testenv.Isolate(t, cliutil.ConfigDir, cliutil.DataDir, cliutil.StateDir, cliutil.CacheDir)
	t.Setenv("DNSFILTER_API_KEY", "")
	t.Setenv("DNSFILTER_CONFIG", "")
	defaultPath, err := cliutil.CredentialsFilePath()
	if err != nil {
		t.Fatal(err)
	}
	if err := cliutil.SaveCredentialsAt(defaultPath, &cliutil.Credentials{DnsfilterApiKey: "default-key"}); err != nil {
		t.Fatal(err)
	}
	tenant := filepath.Join(t.TempDir(), "tenant", "config.toml")
	if err := os.MkdirAll(filepath.Dir(tenant), 0o700); err != nil {
		t.Fatal(err)
	}
	sibling, err := cliutil.CredentialsFilePathForConfig(tenant)
	if err != nil {
		t.Fatal(err)
	}
	if err := cliutil.SaveCredentialsAt(sibling, &cliutil.Credentials{DnsfilterApiKey: "tenant-key"}); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(tenant)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DnsfilterApiKey != "tenant-key" {
		t.Fatalf("explicit config loaded %q, want the sibling's tenant-key", cfg.DnsfilterApiKey)
	}
	if err := cfg.SaveCredential("tenant-key-2"); err != nil {
		t.Fatal(err)
	}
	if cfg, err = Load(tenant); err != nil || cfg.DnsfilterApiKey != "tenant-key-2" {
		t.Fatalf("set-token under an explicit config did not take effect: %q (%v)", cfg.DnsfilterApiKey, err)
	}
	if err := cfg.ClearTokens(); err != nil {
		t.Fatal(err)
	}
	// The sibling now records the logout (explicit-config-logout-never-falls-back)
	// and holds no credential.
	if has, err := cliutil.CredentialsFileHasValuesAt(sibling); err != nil || has {
		t.Fatalf("logout left the explicit config's credentials at %s (%v)", sibling, err)
	}
	if data, _ := os.ReadFile(sibling); strings.Contains(string(data), "tenant-key") {
		t.Fatalf("logout left the tenant key in %s: %q", sibling, data)
	}
	def, err := Load("")
	if err != nil || def.DnsfilterApiKey != "default-key" {
		t.Fatalf("logout under an explicit config touched the default credentials: %q (%v)", def.DnsfilterApiKey, err)
	}
}
