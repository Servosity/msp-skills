// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"connectwise-control-pp-cli/internal/cliutil"
	"connectwise-control-pp-cli/internal/cliutil/testenv"
)

// connectwise-control handfix local-writes-honour-dry-run: --dry-run writes
// no local file, whatever the command.
func TestLocalWritesHonourDryRun(t *testing.T) {
	testenv.Isolate(t)
	t.Setenv("CONNECTWISE_CONTROL_BASE_URL", "http://127.0.0.1:1")
	run := func(args ...string) {
		t.Helper()
		cmd := RootCmd()
		cmd.SetArgs(args)
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		if err := cmd.Execute(); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out.String())
		}
		if args[len(args)-1] == "--dry-run" && !bytes.Contains(out.Bytes(), []byte("dry-run")) {
			t.Fatalf("%v: no dry-run report in %q", args, out.String())
		}
	}

	credPath, err := cliutil.CredentialsFilePath()
	if err != nil {
		t.Fatal(err)
	}
	run("auth", "set-credentials", "dryRunUser-5a1c", "dryRunPass-8f3e2d", "--dry-run")
	if _, err := os.Stat(credPath); !os.IsNotExist(err) {
		t.Fatalf("auth set-credentials --dry-run wrote %s (stat err %v)", credPath, err)
	}

	run("auth", "set-credentials", "realUser-2b7d", "realPass-6c9a1e")
	before, err := os.ReadFile(credPath)
	if err != nil {
		t.Fatal(err)
	}
	run("auth", "logout", "--dry-run")
	if after, _ := os.ReadFile(credPath); !bytes.Equal(before, after) {
		t.Fatal("auth logout --dry-run changed the credentials file")
	}

	outFile := filepath.Join(t.TempDir(), "keep.jsonl")
	if err := os.WriteFile(outFile, []byte("keep me\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	run("export", "session_groups", "-o", outFile, "--dry-run")
	if got, _ := os.ReadFile(outFile); string(got) != "keep me\n" {
		t.Fatalf("export -o --dry-run rewrote the file: %q", got)
	}
}
