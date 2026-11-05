// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// Next-batch round 2 (handfixes.json credkit-wiring): the holder
// is complete before either binary prints anything (a path warning written
// while the config loads), every config the program reads is held, malformed
// or refused ones too, the early scan finds --config past a flag it cannot
// parse, and an exit through the flag package still drains the guard.
// Driven through the built binaries.

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestNB4EveryCredentialIsHeldBeforeAnythingPrints(t *testing.T) {
	if testing.Short() {
		t.Skip("builds both binaries")
	}
	cliBin, mcpBin := nb2Binaries(t)
	without := func(env []string, names ...string) []string {
		var kept []string
		for _, kv := range env {
			name, _, _ := strings.Cut(kv, "=")
			drop := false
			for _, n := range names {
				drop = drop || name == n
			}
			if !drop {
				kept = append(kept, kv)
			}
		}
		return kept
	}
	writeFile := func(t *testing.T, path, body string, mode os.FileMode) {
		t.Helper()
		if mode&0o077 != 0 && runtime.GOOS == "windows" {
			t.Skip("a loose file is a mode-bit case; Windows has no such bits")
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), mode); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(path, mode); err != nil {
			t.Fatal(err)
		}
	}
	cases := []struct {
		name  string
		bin   string
		v     string
		keep  string
		setup func(t *testing.T, home string, env []string, v string) ([]string, []string)
	}{
		{"cli-config-dir-warning", cliBin, "review-secret-0007", "credkit: withheld", func(t *testing.T, home string, env []string, v string) ([]string, []string) {
			return append(without(env, "HUBSPOT_CONFIG_DIR"), "HUBSPOT_ACCESS_TOKEN="+v, "HUBSPOT_CONFIG_DIR="+v), []string{"--help"}
		}},
		{"mcp-config-dir-warning", mcpBin, "review-secret-0012", "credkit: withheld", func(t *testing.T, home string, env []string, v string) ([]string, []string) {
			return append(without(env, "HUBSPOT_CONFIG_DIR"), "HUBSPOT_ACCESS_TOKEN="+v, "HUBSPOT_CONFIG_DIR="+v), []string{"--transport", "bogus"}
		}},
		{"cli-help-value-then-config", cliBin, "review-secret-0008", "credkit: withheld", func(t *testing.T, home string, env []string, v string) ([]string, []string) {
			path := filepath.Join(home, "hs-review.toml")
			writeFile(t, path, "access_token = \""+v+"\"\n", 0o600)
			return env, []string{"--help=" + v, "--config", path}
		}},
		{"cli-short-help-value-then-config-equals", cliBin, "review-secret-0009", "credkit: withheld", func(t *testing.T, home string, env []string, v string) ([]string, []string) {
			path := filepath.Join(home, "hs-review.toml")
			writeFile(t, path, "access_token = \""+v+"\"\n", 0o600)
			return env, []string{"-h=" + v, "--config=" + path}
		}},
		{"cli-malformed-config", cliBin, "review-secret-0010", "credkit: withheld", func(t *testing.T, home string, env []string, v string) ([]string, []string) {
			writeFile(t, filepath.Join(nb2ConfigDir(home), "config.toml"), "access_token = \""+v+"\n", 0o600)
			return env, []string{v}
		}},
		{"cli-refused-loose-config", cliBin, "review-secret-0011", "credkit: withheld", func(t *testing.T, home string, env []string, v string) ([]string, []string) {
			writeFile(t, filepath.Join(nb2ConfigDir(home), "config.toml"), "access_token = \""+v+"\"\n\n[headers]\nX-Extra = \"Bearer "+v+"-hdr\"\n", 0o644)
			return env, []string{v + "-hdr"}
		}},
		{"cli-refused-loose-credentials-file", cliBin, "review-secret-0015", "credkit: withheld", func(t *testing.T, home string, env []string, v string) ([]string, []string) {
			writeFile(t, filepath.Join(home, "data", "credentials.toml"), "access_token = \""+v+"\"\n", 0o644)
			return env, []string{v}
		}},
		{"cli-malformed-credentials-file", cliBin, "review-secret-0016", "credkit: withheld", func(t *testing.T, home string, env []string, v string) ([]string, []string) {
			writeFile(t, filepath.Join(home, "data", "credentials.toml"), "access_token = \""+v+"\n", 0o600)
			return env, []string{v}
		}},
		{"cli-legacy-malformed-config", cliBin, "review-secret-0013", "credkit: withheld", func(t *testing.T, home string, env []string, v string) ([]string, []string) {
			writeFile(t, filepath.Join(home, ".config", "hubspot-cli", "config.toml"), "access_token = \""+v+"\n", 0o600)
			return without(env, "HUBSPOT_CONFIG_DIR", "HUBSPOT_HOME"), []string{v}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			env, args := tc.setup(t, home, nb2Env(home, "http://127.0.0.1:1", cliBin), tc.v)
			out, stderr, _ := nb2Run(t, tc.bin, env, "", args...)
			nb2AssertNoForms(t, tc.name, out+stderr, nb2Forms(tc.v))
			if !strings.Contains(out+stderr, tc.keep) {
				t.Errorf("%s want the kit's notice %q:\n%s%s", tc.name, tc.keep, out, stderr)
			}
		})
	}
}

// An exit the flag package takes itself (-h, an unknown flag) still drains
// the guard, every time.
func TestNB4MCPFlagExitsDrainTheGuard(t *testing.T) {
	if testing.Short() {
		t.Skip("builds both binaries")
	}
	cliBin, mcpBin := nb2Binaries(t)
	for _, tc := range []struct {
		args []string
		keep string
	}{{[]string{"-h"}, "-transport"}, {[]string{"--not-a-flag"}, "flag provided but not defined"}} {
		for i := 0; i < 5; i++ {
			home := t.TempDir()
			out, stderr, _ := nb2Run(t, mcpBin, nb2Env(home, "http://127.0.0.1:1", cliBin, "HUBSPOT_ACCESS_TOKEN=review-secret-0014"), "", tc.args...)
			if !strings.Contains(out+stderr, tc.keep) {
				t.Fatalf("hubspot-mcp %v run %d lost %q:\n%s%s", tc.args, i, tc.keep, out, stderr)
			}
		}
	}
}
