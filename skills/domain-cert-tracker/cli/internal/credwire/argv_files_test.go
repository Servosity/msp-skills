// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated): Codex publish pass 2 round 2,
// startup-config-discovery. Declarations.Files names every file the command
// line could mean, so a flag's value that looks like a flag cannot hide the
// file the program reads.

package credwire

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// isolatedEnv points every path lookup at a throwaway home.
func isolatedEnv(t *testing.T) string {
	t.Helper()
	root, err := os.MkdirTemp("", "dct-argv-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		if strings.HasPrefix(name, "XDG_") || strings.HasPrefix(name, "DOMAIN_CERT_TRACKER_") || strings.HasPrefix(name, "PRINTING_PRESS_") {
			t.Setenv(name, "")
			_ = os.Unsetenv(name)
		}
	}
	t.Setenv("HOME", root)
	t.Setenv("USERPROFILE", root)
	return root
}

func TestCredentialFilesNamesEveryCandidate(t *testing.T) {
	root := isolatedEnv(t)
	real := filepath.Join(root, "real")
	fake := filepath.Join(root, "fake")
	realCfg := filepath.Join(real, "config", "config.toml")
	fakeCfg := filepath.Join(fake, "config", "config.toml")
	if err := os.MkdirAll(filepath.Dir(realCfg), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(realCfg, []byte("auth_header = \"x\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	a, b := filepath.Join(root, "a.toml"), filepath.Join(root, "b.toml")

	cases := []struct {
		name  string
		args  []string
		env   map[string]string
		first string   // the file the program reads, named first
		all   []string // every file that must be named
		none  []string // files that must not be named
	}{
		{
			name: "a flag value that looks like --home (the reviewer's input)",
			args: []string{"--home", real, "teach", "--notes", "--home=" + fake, "--query", "q", "--resource-type", "domain", "--resource", "example.com"},
			all:  []string{realCfg, fakeCfg},
		},
		{
			name: "a flag value that looks like --config",
			args: []string{"--config", a, "teach", "--notes", "--config=" + b, "--query", "q", "--resource-type", "domain", "--resource", "example.com"},
			all:  []string{a, b},
		},
		{
			// Adversarial round 2: the program reads only --config here, so
			// the default file must not be held. The kit takes the base URL from
			// the last --config itself.
			name:  "a --config flag leaves the default file out",
			args:  []string{"--config", a, "sweep"},
			first: a, all: []string{a}, none: []string{"default"},
		},
		{
			name:  "--config beats --home",
			args:  []string{"--home", real, "--config", a, "sweep"},
			first: a, all: []string{a, realCfg},
		},
		{
			name:  "the last --home wins",
			args:  []string{"--home", fake, "teach", "--home=" + real, "--notes", "n", "--query", "q", "--resource-type", "domain", "--resource", "example.com"},
			first: realCfg, all: []string{realCfg},
		},
		{
			name: "a command line that does not parse names every candidate",
			args: []string{"--home", real, "--no-such-flag", "--home=" + fake, "sweep"},
			all:  []string{realCfg, fakeCfg},
		},
		{
			name: "an unknown command names every candidate",
			args: []string{"--home=" + fake, "no-such-command", "--home", real},
			all:  []string{realCfg, fakeCfg},
		},
		{
			name:  "the MCP server's command line reads the config environment",
			args:  []string{"--transport", "http", "--addr", "127.0.0.1:7777"},
			env:   map[string]string{"DOMAIN_CERT_TRACKER_CONFIG": a},
			first: a, all: []string{a},
		},
		{
			name:  "the MCP server's command line reads DOMAIN_CERT_TRACKER_HOME",
			args:  []string{"--transport", "stdio"},
			env:   map[string]string{"DOMAIN_CERT_TRACKER_HOME": real},
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
			if tc.none != nil && len(got) != 1 {
				t.Errorf("named more than the one file the program reads: %q", got)
			}
		})
	}
}

// The reviewer's input through the real binary: auth_header in
// <real>/config/config.toml, no config environment variable and no fake or
// legacy config, then
//
//	domain-cert-tracker-cli --home <real> teach --notes --home=<fake> --query lowercasecredential123 --resource-type domain --resource example.com
//
// Cobra gives "--home=<fake>" to --notes, so the program reads <real>'s
// config; the credential in it must be held, so the query that carries it
// never reaches <real>/state/learnings.jsonl.
func TestKitStartupConfigDiscoveryHonoursFlagValues(t *testing.T) {
	cli, _ := binaries(t)
	const secret = "lowercasecredential123"
	s := newSandbox(t)
	real := filepath.Join(s.root, "real")
	fake := filepath.Join(s.root, "fake")
	cfg := filepath.Join(real, "config", "config.toml")
	if err := os.MkdirAll(filepath.Dir(cfg), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg, []byte("auth_header=\""+secret+"\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var env []string
	for _, kv := range s.env() {
		if !strings.HasPrefix(kv, "DOMAIN_CERT_TRACKER_HOME=") {
			env = append(env, kv)
		}
	}
	args := []string{"--home", real, "teach", "--notes", "--home=" + fake, "--query", secret, "--resource-type", "domain", "--resource", "example.com"}
	res := run(t, cli, env, args...)
	requireAbsent(t, "teach stdout", res.stdout, secret)
	requireAbsent(t, "teach stderr", res.stderr, secret)
	if hits := filesHolding(t, s.root, secret, cfg); len(hits) > 0 {
		t.Errorf("teach persisted the credential to %v (exit %d, stderr=%q)", hits, res.code, res.stderr)
	}
	// The same command with an ordinary query writes it, so the absence
	// above is the credential being held, not a teach that writes nothing.
	// (teach stores the query normalized: "an" is dropped.)
	args[6] = "an ordinary query"
	res = run(t, cli, env, args...)
	if hits := filesHolding(t, filepath.Join(real, "state"), "ordinary query"); len(hits) == 0 {
		t.Errorf("teach wrote nothing under <real>/state even without a credential, so this test proved nothing (exit %d, stderr=%q)", res.code, res.stderr)
	}
}
