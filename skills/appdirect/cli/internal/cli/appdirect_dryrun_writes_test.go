// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// appdirect handfix dry-run-changes-nothing (press-traps s6): a --dry-run
// preview of a command that writes local files writes nothing. 4.34.2's
// `auth set-token --dry-run` saved the token, `auth logout --dry-run` cleared
// the stored credentials, and `export --output f --dry-run` replaced f with
// the preview.
func TestDryRunChangesNoLocalFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("APPDIRECT_HOME", dir)
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(dir, "data"))
	t.Setenv("APPDIRECT_BASE_URL", "http://127.0.0.1:9")
	run := func(stdin string, args ...string) {
		t.Helper()
		cmd := newRootCmd(&rootFlags{})
		var out, errb bytes.Buffer
		cmd.SetIn(strings.NewReader(stdin))
		cmd.SetOut(&out)
		cmd.SetErr(&errb)
		cmd.SetArgs(args)
		if err := cmd.Execute(); err != nil {
			t.Fatalf("%v: %v\n%s%s", args, err, out.String(), errb.String())
		}
	}
	snapshot := func() map[string]string {
		m := map[string]string{}
		_ = filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
			if err == nil && !info.IsDir() && !strings.HasSuffix(p, ".lock") {
				b, _ := os.ReadFile(p)
				m[p] = string(b)
			}
			return nil
		})
		return m
	}

	run("dry-token-0123456789\n", "auth", "set-token", "--dry-run")
	if creds := filepath.Join(dir, "data", "credentials.toml"); fileExists(creds) {
		t.Fatalf("auth set-token --dry-run saved the token (%s)", creds)
	}
	run("kept-token-0123456789\n", "auth", "set-token")
	before := snapshot()
	run("", "auth", "logout", "--dry-run")
	out := filepath.Join(dir, "export.jsonl")
	if err := os.WriteFile(out, []byte("keep\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	before[out] = "keep\n"
	run("", "export", "account", "--output", out, "--dry-run")
	after := snapshot()
	for p, v := range before {
		if after[p] != v {
			t.Errorf("a --dry-run changed %s", p)
		}
	}
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
