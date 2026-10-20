// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/dnsfilter/handfixes.json
// under the "config-env-base-url-never-persisted" entry.

package config

import (
	"path/filepath"
	"testing"

	"dnsfilter-pp-cli/internal/cliutil"
	"dnsfilter-pp-cli/internal/cliutil/testenv"
)

// TestEnvBaseURLNeverPersisted: saving a token while DNSFILTER_BASE_URL
// overrides the base URL for one process writes the file's own base URL, so
// a later run without the override still sends the token to the DNSFilter API.
func TestEnvBaseURLNeverPersisted(t *testing.T) {
	testenv.Isolate(t, cliutil.ConfigDir, cliutil.DataDir, cliutil.StateDir, cliutil.CacheDir)
	path := filepath.Join(t.TempDir(), "config.toml")
	t.Setenv("DNSFILTER_BASE_URL", "http://127.0.0.1:9/evil")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.BaseURL != "http://127.0.0.1:9/evil" {
		t.Fatalf("override not applied: %q", cfg.BaseURL)
	}
	if err := cfg.SaveCredential("test-token"); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DNSFILTER_BASE_URL", "")
	cfg, err = Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.BaseURL != "https://api.dnsfilter.com" {
		t.Fatalf("a one-process base URL override was saved: %q", cfg.BaseURL)
	}
}
