// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package config

import (
	"path/filepath"
	"testing"

	"watchguard-pp-cli/internal/cliutil"
)

// isolateCredentialHome points every press directory, and the legacy home, at
// one temp dir. USERPROFILE is set too: os.UserHomeDir reads it on Windows.
func isolateCredentialHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("WATCHGUARD_HOME", home)
	for _, name := range []string{"WATCHGUARD_CONFIG", "WATCHGUARD_BASE_URL", "WATCHGUARD_TOKEN_URL",
		"WATCHGUARD_CLIENT_ID", "WATCHGUARD_CLIENT_SECRET", "WATCHGUARD_API_KEY"} {
		t.Setenv(name, "")
	}
	return home
}

// TestGlobalCredentialsStayOnTheirIPv6Zone (handfixes.json
// stored-credential-origin-exact-host, print Codex r2 [P1] "credential sent to
// another host"): global credentials recorded for https://[fe80::1%25lan0] must
// not be applied to an explicit --config profile whose base_url names
// https://[fe80::1%25LAN0]. Interface names are case-sensitive on Linux, so the
// two zones are two links and possibly two peers; the comparator folded the
// zone's case and sent the saved token and API key to the other one. The same
// zone spelled the same way still applies them (the control).
func TestGlobalCredentialsStayOnTheirIPv6Zone(t *testing.T) {
	const token = "saved-" + "zoneTok-7q2w9e"
	const apiKey = "saved-" + "zoneKey-4r8t1y"
	cases := []struct {
		name, profileBase string
		applied           bool
	}{
		{"other-zone-case", "https://[fe80::1%25LAN0]", false},
		{"same-zone", "https://[fe80::1%25lan0]", true},
		{"same-zone-default-port", "https://[fe80::1%25lan0]:443/", true},
		{"other-zone", "https://[fe80::1%25lan1]", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			isolateCredentialHome(t)
			if err := cliutil.SaveCredentials(&cliutil.Credentials{
				BaseURL:          "https://[fe80::1%25lan0]",
				AccessToken:      token,
				WatchguardApiKey: apiKey,
			}); err != nil {
				t.Fatal(err)
			}
			profile := filepath.Join(t.TempDir(), "profile.toml")
			if err := cliutil.AtomicWriteCredentialFile(profile, []byte("base_url = \""+c.profileBase+"\"\n"), 0o600, 0o700); err != nil {
				t.Fatal(err)
			}
			cfg, err := Load(profile)
			if err != nil {
				t.Fatal(err)
			}
			got := cfg.AccessToken == token || cfg.WatchguardApiKey == apiKey
			if got != c.applied {
				t.Errorf("profile base_url %s: global credentials for https://[fe80::1%%25lan0] applied=%v, want %v (refusals: %v)",
					c.profileBase, got, c.applied, cfg.CredentialRefusalSummaries())
			}
		})
	}
}
