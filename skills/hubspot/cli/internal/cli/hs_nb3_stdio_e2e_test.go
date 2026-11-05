// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// Next-batch round 1 (handfixes.json credkit-wiring): every byte
// either binary writes passes the chokepoint, including what a command writes
// to os.Stdout or os.Stderr directly (the dry-run request body) and what is
// written before Cobra finishes parsing (an unknown command), and every
// credential is held before that. Driven through the built binaries.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNB3EveryByteTheBinariesWriteIsWithheld(t *testing.T) {
	if testing.Short() {
		t.Skip("builds both binaries")
	}
	cliBin, mcpBin := nb2Binaries(t)
	const base = "pat-na1-" + "e2e7f3c0-9a6b-4c8e-8d1f-2b5e7c1a9d30"
	dryRun := func(firstname string) []string {
		return []string{"hubspot-contacts-crm", "post-v3-objects-contacts", "--associations", "[]", "--properties", `{"firstname":"` + firstname + `"}`, "--dry-run"}
	}
	cases := []struct {
		name  string
		bin   string
		forms map[string]string
		keep  string
		setup func(t *testing.T, home string, env []string) ([]string, []string)
	}{
		{"cli-dry-run-body", cliBin, nb2Forms("review-secret-0001"), "credkit: withheld", func(t *testing.T, home string, env []string) ([]string, []string) {
			return append(env, "HUBSPOT_ACCESS_TOKEN=review-secret-0001"), dryRun("review-secret-0001")
		}},
		{"cli-dry-run-mixed-case", cliBin, map[string]string{"mixed": "rEvIeW-sEcReT-0003"}, "credkit: withheld", func(t *testing.T, home string, env []string) ([]string, []string) {
			return append(env, "HUBSPOT_ACCESS_TOKEN=review-secret-0003"), dryRun("rEvIeW-sEcReT-0003")
		}},
		{"cli-dry-run-short-base32hex", cliBin, map[string]string{"base32hex": "B9OJE"}, "credkit: withheld", func(t *testing.T, home string, env []string) ([]string, []string) {
			nb2WriteConfig(t, home, "access_token = \""+base+"\"\n\n[headers]\nX-Api-Key = \"Zq7\"\n")
			return env, dryRun("B9OJE")
		}},
		{"cli-unknown-command-env", cliBin, nb2Forms("review-secret-0002"), "credkit: withheld", func(t *testing.T, home string, env []string) ([]string, []string) {
			return append(env, "HUBSPOT_ACCESS_TOKEN=review-secret-0002"), []string{"review-secret-0002"}
		}},
		{"cli-unknown-command-config-flag", cliBin, nb2Forms("review-secret-0004"), "credkit: withheld", func(t *testing.T, home string, env []string) ([]string, []string) {
			path := filepath.Join(home, "elsewhere.toml")
			if err := os.WriteFile(path, []byte("access_token = \"review-secret-0004\"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			return env, []string{"--config", path, "review-secret-0004"}
		}},
		{"cli-unknown-command-home-flag", cliBin, nb2Forms("review-secret-0005"), "credkit: withheld", func(t *testing.T, home string, env []string) ([]string, []string) {
			dir := filepath.Join(home, "alt")
			if err := os.MkdirAll(filepath.Join(dir, "config"), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "config", "config.toml"), []byte("access_token = \"review-secret-0005\"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			var kept []string
			for _, kv := range env {
				if !strings.HasPrefix(kv, "HUBSPOT_HOME=") && !strings.HasPrefix(kv, "HUBSPOT_CONFIG_DIR=") {
					kept = append(kept, kv)
				}
			}
			return kept, []string{"--home", dir, "review-secret-0005"}
		}},
		{"mcp-unknown-transport", mcpBin, nb2Forms("review-secret-0006"), "credkit: withheld", func(t *testing.T, home string, env []string) ([]string, []string) {
			return append(env, "HUBSPOT_ACCESS_TOKEN=review-secret-0006"), []string{"--transport", "review-secret-0006"}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			env, args := tc.setup(t, home, nb2Env(home, "http://127.0.0.1:1", cliBin))
			out, stderr, _ := nb2Run(t, tc.bin, env, "", args...)
			nb2AssertNoForms(t, tc.name, out+stderr, tc.forms)
			if !strings.Contains(out+stderr, tc.keep) {
				t.Errorf("%s want the kit's notice %q:\n%s%s", tc.name, tc.keep, out, stderr)
			}
		})
	}
}
