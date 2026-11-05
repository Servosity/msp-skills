// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): the adopt pass on the zammad 4.34.0 publish
// (2026-10-28). Pinned by this connector's handfixes.json under the
// "credkit-wiring" entry.

package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"zammad-pp-cli/internal/cliutil/testenv"
)

// adoptRunExecute runs Execute with args and returns everything it wrote
// to stdout and stderr, and its exit code.
func adoptRunExecute(t *testing.T, args ...string) (string, int) {
	t.Helper()
	savedArgs, savedOut, savedErr := os.Args, os.Stdout, os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Args = append([]string{"zammad-cli"}, args...)
	os.Stdout, os.Stderr = w, w
	done := make(chan string)
	go func() {
		var b bytes.Buffer
		_, _ = b.ReadFrom(r)
		done <- b.String()
	}()
	code := 0
	if err := Execute(); err != nil {
		code = ExitCode(err)
	}
	w.Close()
	os.Args, os.Stdout, os.Stderr = savedArgs, savedOut, savedErr
	return <-done, code
}

// TestAdoptBrokenConfigIsRefused: a configuration file that did not parse
// was read by a line heuristic, and each review found the next shape it
// missed: a URL inside a multi-line string, a value with no letter or
// digit (Codex nb-r2 [P1], repeating nb-r1-1). The class: a configuration
// or credentials file that does not parse is refused at startup, before
// anything is parsed or printed, with a message that names the path and
// never quotes the file; no value from it can reach output.
func TestAdoptBrokenConfigIsRefused(t *testing.T) {
	home := testenv.Isolate(t)
	cases := map[string]struct{ body, token string }{
		"multi-line base_url":  {"base_url = \"\"\"\nhttps://op:%s@zammad.example.com/api/v1\"\"\"\nbroken = \"unterminated\n", "abu-" + strings.Repeat("Ub7q", 12)},
		"punctuation only":     {"api_token = \"%s\" # saved\nbroken = \"unterminated\n", "!!!!!!!!"},
		"quoted with comment":  {"api_token = \"%s\" # saved\nbroken = \"unterminated\n", "abq-" + strings.Repeat("Qc4m", 12)},
		"multi-line string":    {"api_token = \"\"\"\n%s\n\"\"\"\nbroken = \"unterminated\n", "abm-" + strings.Repeat("Mc7m", 12)},
		"header with comment":  {"[headers]\nX-Api-Key = \"%s\" # key\nbroken = \"unterminated\n", "abh-" + strings.Repeat("Hc8m", 12)},
		"array of credentials": {"api_token = [\n\"%s\",\nbroken = \"unterminated\n", "aba-" + strings.Repeat("Ac2m", 12)},
	}
	for name, c := range cases {
		path := filepath.Join(home, strings.ReplaceAll(name, " ", "-")+".toml")
		if err := os.WriteFile(path, []byte(strings.Replace(c.body, "%s", c.token, 1)), 0o600); err != nil {
			t.Fatal(err)
		}
		for _, args := range [][]string{
			{"--config", path, "--no-learn", "--" + c.token},
			{"--config", path, "--no-learn", "tickets", "get", c.token},
			{"--config", path, "--no-learn", "version"},
		} {
			out, code := adoptRunExecute(t, args...)
			if strings.Contains(out, c.token) {
				t.Errorf("%s %v: output carries the broken file's value: %s", name, args[3:], out)
			}
			if code == 0 || !strings.Contains(out, path) {
				t.Errorf("%s %v: not refused naming the file (exit %d): %s", name, args[3:], code, out)
			}
		}
	}
	// The same file, fixed, runs.
	clean := filepath.Join(home, "clean.toml")
	if err := os.WriteFile(clean, []byte("base_url = \"https://zammad.example.com/api/v1\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if out, code := adoptRunExecute(t, "--config", clean, "--no-learn", "version"); code != 0 || strings.Contains(out, "does not parse") {
		t.Errorf("a valid config was refused (exit %d): %s", code, out)
	}
}
