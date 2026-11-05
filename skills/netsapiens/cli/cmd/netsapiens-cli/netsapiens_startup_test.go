// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Class test (credential-output, startup ordering), the built CLI: a config or
// credentials file that fails to parse still has every value in it held before
// any startup diagnostic can print, whatever its size. The Codex round-1 input
// is a legacy config whose duplicate key equals the credential, with --help.

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"netsapiens-pp-cli/internal/cliutil/testenv"
)

const startupSecret = "StartupSecret55443322"

// malformedTOML names the credential as a duplicate key, so the parser's
// diagnostic ("key ... is already defined") repeats it. table puts the
// duplicate under [headers] (a config map); otherwise it is top-level (the
// credentials file has no map to hold it, and its parser skips unknown tables).
func malformedTOML(pad int, table bool) string {
	dup := startupSecret + " = \"a\"\n" + startupSecret + " = \"b\"\n"
	if table {
		dup = "[headers]\n" + dup
	}
	return strings.Repeat("# padding line before the credential\n", pad) + "api_key = \"" + startupSecret + "\"\n" + dup
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestClassMalformedConfigNeverPrintsACredential(t *testing.T) {
	cliBinary(t) // before Isolate: the build needs the real module and build caches
	cases := []struct {
		name  string
		place func(home string) []string // writes the files, returns extra env
		args  []string
	}{
		{"legacy config, --help (Codex round 1)", func(home string) []string {
			writeFile(t, filepath.Join(home, ".config", "netsapiens-cli", "config.toml"), malformedTOML(0, true))
			return nil
		}, []string{"--help"}},
		{"legacy config, credential past the kit stream window, --help", func(home string) []string {
			writeFile(t, filepath.Join(home, ".config", "netsapiens-cli", "config.toml"), malformedTOML(1000, true))
			return nil
		}, []string{"--help"}},
		{"config file, doctor", func(home string) []string {
			writeFile(t, filepath.Join(home, "cfg", "config.toml"), malformedTOML(0, true))
			return nil
		}, []string{"doctor", "--json"}},
		{"credentials file, --help", func(home string) []string {
			writeFile(t, filepath.Join(home, "data", "credentials.toml"), malformedTOML(0, false))
			return nil
		}, []string{"--help"}},
		{"credentials file, doctor", func(home string) []string {
			writeFile(t, filepath.Join(home, "data", "credentials.toml"), malformedTOML(0, false))
			return nil
		}, []string{"doctor", "--json"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			home := testenv.Isolate(t)
			env := []string{"NETSAPIENS_API_KEY=", "NETSAPIENS_CONFIG_DIR=" + filepath.Join(home, "cfg"), "NETSAPIENS_DATA_DIR=" + filepath.Join(home, "data")}
			env = append(env, tc.place(home)...)
			r := runCLI(t, env, tc.args...)
			if f := shown(r.stdout+r.stderr, startupSecret); f != "" {
				t.Fatalf("%q reached the terminal:\nstdout=%s\nstderr=%s", f, r.stdout, r.stderr)
			}
		})
	}
}
