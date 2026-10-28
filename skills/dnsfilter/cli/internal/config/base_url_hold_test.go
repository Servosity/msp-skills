// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/dnsfilter/handfixes.json under
// the "base-url-credentials-held-and-redacted" entry (class "secret-echo").

package config

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"dnsfilter-pp-cli/internal/cliutil"
	"dnsfilter-pp-cli/internal/cliutil/testenv"
	"dnsfilter-pp-cli/internal/credkit"
)

func baseURLHeld(v string) bool {
	return errors.Is(credkit.WriteFile(filepath.Join(os.TempDir(), "dnsfilter-held-"+strconv.Itoa(os.Getpid())), []byte("before "+v+" after"), 0o600), credkit.ErrWithheld)
}

func writeHoldFixture(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// malformedConfig is a config file TOML rejects that carries a base URL
// password, an api_key and a [headers] value, each named by tag.
func malformedConfig(tag string) (string, []string) {
	pw, key, hdr := tag+"UrlPw4242", tag+"ApiKey4242", tag+"Header4242"
	return "base_url = \"https://u:" + pw + "@127.0.0.1:9\"\napi_key = \"" + key + "\"\n[headers]\nX-Org = \"" + hdr + "\"\nbroken = = 1\n", []string{pw, key, hdr}
}

// TestConfigLoadHoldsBaseURLCredentials: Load holds the credentials of the
// base URL before anything can print it, from every config file whose bytes it
// reads, even one TOML rejects (explicit, default and legacy paths). The kit
// holds DNSFILTER_BASE_URL at startup (internal/credwire).
func TestConfigLoadHoldsBaseURLCredentials(t *testing.T) {
	t.Run("explicit-malformed", func(t *testing.T) {
		home := testenv.Isolate(t, cliutil.ConfigDir, cliutil.DataDir, cliutil.StateDir, cliutil.CacheDir)
		path := filepath.Join(home, "tenant", "config.toml")
		body, secrets := malformedConfig("Explicit")
		writeHoldFixture(t, path, body)
		if _, err := Load(path); err == nil {
			t.Fatal("a config TOML rejects loaded")
		}
		for _, s := range secrets {
			if !baseURLHeld(s) {
				t.Errorf("an explicit config TOML rejects did not hold %q", s)
			}
		}
	})
	t.Run("default-malformed", func(t *testing.T) {
		testenv.Isolate(t, cliutil.ConfigDir, cliutil.DataDir, cliutil.StateDir, cliutil.CacheDir)
		dir, err := cliutil.ConfigDir()
		if err != nil {
			t.Fatal(err)
		}
		body, secrets := malformedConfig("Default")
		writeHoldFixture(t, filepath.Join(dir, "config.toml"), body)
		_, _ = Load("")
		for _, s := range secrets {
			if !baseURLHeld(s) {
				t.Errorf("the default config TOML rejects did not hold %q", s)
			}
		}
	})
	t.Run("legacy-malformed", func(t *testing.T) {
		testenv.Isolate(t, cliutil.ConfigDir, cliutil.DataDir, cliutil.StateDir, cliutil.CacheDir)
		legacy, err := LegacyConfigPath()
		if err != nil {
			t.Fatal(err)
		}
		body, secrets := malformedConfig("Legacy")
		writeHoldFixture(t, legacy, body)
		if _, err := Load(""); err != nil {
			t.Fatalf("a legacy config TOML rejects is skipped, not an error: %v", err)
		}
		for _, s := range secrets {
			if !baseURLHeld(s) {
				t.Errorf("the legacy config TOML rejects did not hold %q", s)
			}
		}
	})
}

// TestConfigFileReadsHoldTheirBytes: every other reader of a config file's
// bytes holds them too: the credential probe, the save-time comparison and
// the legacy scrub.
func TestConfigFileReadsHoldTheirBytes(t *testing.T) {
	home := testenv.Isolate(t, cliutil.ConfigDir, cliutil.DataDir, cliutil.StateDir, cliutil.CacheDir)
	t.Run("credential-probe", func(t *testing.T) {
		path := filepath.Join(home, "probe", "config.toml")
		body, secrets := malformedConfig("Probe")
		writeHoldFixture(t, path, body)
		_, _ = FileHasCredentialFields(path)
		for _, s := range secrets {
			if !baseURLHeld(s) {
				t.Errorf("FileHasCredentialFields did not hold %q", s)
			}
		}
	})
	t.Run("save-comparison", func(t *testing.T) {
		path := filepath.Join(home, "compare", "config.toml")
		body, secrets := malformedConfig("Compare")
		writeHoldFixture(t, path, body)
		_ = configFileAlreadyHolds(path, persistedConfig{})
		for _, s := range secrets {
			if !baseURLHeld(s) {
				t.Errorf("configFileAlreadyHolds did not hold %q", s)
			}
		}
	})
	t.Run("legacy-scrub", func(t *testing.T) {
		path := filepath.Join(home, "scrub", "config.toml")
		body, secrets := malformedConfig("Scrub")
		writeHoldFixture(t, path, body)
		c := &Config{Path: filepath.Join(home, "other", "config.toml"), legacySourcePath: path}
		c.scrubLegacyCredentials()
		for _, s := range secrets {
			if !baseURLHeld(s) {
				t.Errorf("scrubLegacyCredentials did not hold %q", s)
			}
		}
	})
}
