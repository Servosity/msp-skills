// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated; handfixes.json credkit-wiring): the wiring
// file reads the command line the way the program will, so the file whose
// base_url the run uses is the one the kit takes its origin from.

package credwire

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"microsoft-graph-pp-cli/internal/cliutil"
)

// isolatedEnv points every path lookup at a throwaway home.
func isolatedEnv(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		if strings.HasPrefix(name, "XDG_") || strings.HasPrefix(name, "MICROSOFT_GRAPH_") || strings.HasPrefix(name, "PRINTING_PRESS_") {
			t.Setenv(name, "")
			_ = os.Unsetenv(name)
		}
	}
	t.Setenv("HOME", root)
	t.Setenv("USERPROFILE", root)
	return root
}

// configUnder is the config.toml the program reads under --home home.
func configUnder(t *testing.T, home string) string {
	t.Helper()
	restore, err := cliutil.SetHomeOverride(home)
	if err != nil {
		t.Fatal(err)
	}
	defer restore()
	dir, err := cliutil.ConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(dir, "config.toml")
}

func TestCredentialFilesReadsArgvLikeCobra(t *testing.T) {
	root := isolatedEnv(t)
	real := filepath.Join(root, "real")
	fake := filepath.Join(root, "fake")
	realCfg, fakeCfg := configUnder(t, real), configUnder(t, fake)
	a, b := filepath.Join(root, "a.toml"), filepath.Join(root, "b.toml")
	teach := []string{"--query", "q", "--resource-type", "users", "--resource", "123"}
	cases := []struct {
		name  string
		args  []string
		env   map[string]string
		first string   // the file the program reads, named first
		all   []string // every file that must be named
	}{
		{
			name:  "a flag value that looks like --home",
			args:  append([]string{"--home", real, "teach", "--notes", "--home=" + fake}, teach...),
			first: realCfg, all: []string{realCfg, fakeCfg},
		},
		{
			name:  "a flag value that looks like --config",
			args:  append([]string{"--config", a, "teach", "--notes", "--config=" + b}, teach...),
			first: a, all: []string{a, b},
		},
		{
			name:  "the last --home wins",
			args:  append([]string{"--home", fake, "teach", "--home=" + real, "--notes", "n"}, teach...),
			first: realCfg, all: []string{realCfg, fakeCfg},
		},
		{
			name: "a command line that does not parse names every candidate",
			args: []string{"--home", real, "--no-such-flag", "--home=" + fake, "users", "list"},
			all:  []string{realCfg, fakeCfg},
		},
		{
			name:  "the MCP server's command line reads the config environment",
			args:  []string{"--transport", "http", "--addr", "127.0.0.1:7777"},
			env:   map[string]string{"MICROSOFT_GRAPH_CONFIG": a},
			first: a, all: []string{a},
		},
		{
			name:  "the MCP server's command line reads MICROSOFT_GRAPH_HOME",
			args:  []string{"--transport", "stdio"},
			env:   map[string]string{"MICROSOFT_GRAPH_HOME": real},
			first: realCfg, all: []string{realCfg},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			got := credentialFiles(tc.args)
			if len(got) == 0 {
				t.Fatalf("no file named for %q", tc.args)
			}
			if tc.first != "" && got[0] != tc.first {
				t.Errorf("first file %q, want %q (the one the program reads); all: %q", got[0], tc.first, got)
			}
			for _, w := range tc.all {
				if !slices.Contains(got, w) {
					t.Errorf("%q not named; got %q", w, got)
				}
			}
		})
	}
}

// graph-clientid-is-public: the app registration's client id is an identifier
// Graph answers and consent URLs repeat, so no response field holds it; the
// client secret and the --client-id flag stay held and Entra-only.
func TestClientIDIsPublicAndTheSecretIsNot(t *testing.T) {
	for _, f := range Declarations.ResponseFields {
		if strings.EqualFold(f, "clientId") || strings.EqualFold(f, "client_id") {
			t.Errorf("ResponseFields holds %q, a public identifier", f)
		}
	}
	var public bool
	for _, id := range Declarations.Identifiers {
		public = public || id.Source == "config:client_id"
	}
	if !public {
		t.Error("config:client_id is not declared an identifier")
	}
	for _, flag := range []string{"--client-secret", "--client-id"} {
		if !slices.Contains(Declarations.CredentialFlags, flag) {
			t.Errorf("CredentialFlags lacks %s", flag)
		}
		var entraOnly bool
		for _, o := range Declarations.Origins {
			entraOnly = entraOnly || string(o.Source) == "flag:"+flag && slices.Equal(o.Allowed, []string{entraLoginOrigin})
		}
		if !entraOnly {
			t.Errorf("%s may go somewhere other than %s", flag, entraLoginOrigin)
		}
	}
}
