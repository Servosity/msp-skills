// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// cli-printing-press: novel-scaffold-test
// Novel command scaffold tests. Keep the wiring smoke test and add behavior cases as needed.

package cli

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"keeper-pp-cli/internal/cliutil/testenv"
	"keeper-pp-cli/internal/store"
)

// TestNovelPostureRunsHelpWires smoke-tests that the posture runs command
// resolves at runtime and renders useful --help output. Catches wiring
// regressions (missing AddCommand, panicking RunE on --help, etc.) before
// review. Keep this smoke test when adding behavior-specific cases.
func TestNovelPostureRunsHelpWires(t *testing.T) {
	testenv.Isolate(t)
	cmd := RootCmd()
	cmd.SetArgs([]string{"posture", "runs", "--help"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("posture runs --help error = %v (novel command not wired correctly?)", err)
	}
	help := out.String()
	for _, want := range []string{"Usage:", "runs"} {
		if !strings.Contains(help, want) {
			t.Fatalf("posture runs --help missing %q in output:\n%s", want, help)
		}
	}
}

// terminalWriter answers the CLI's terminal check (isTerminal) with yes.
type terminalWriter struct{ io.Writer }

func (terminalWriter) TerminalOverride() bool { return true }

// A terminal on a fresh store gets a sentence, not a bare "[]"; --json and
// piped output stay a JSON array (never null).
func TestPostureRunsEmptyStoreOutput(t *testing.T) {
	testenv.Isolate(t)
	var tty bytes.Buffer
	cmd := RootCmd()
	cmd.SetOut(terminalWriter{&tty})
	cmd.SetErr(&tty)
	cmd.SetArgs([]string{"posture", "runs"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(tty.String()); got != "No posture runs stored yet. Run 'keeper-cli posture report' first." {
		t.Fatalf("terminal output = %q", got)
	}
	for _, args := range [][]string{{"posture", "runs", "--json"}, {"posture", "runs"}} {
		out, err := runKeeperCLI(t, args...)
		if err != nil {
			t.Fatal(err)
		}
		if strings.TrimSpace(out) != "[]" {
			t.Fatalf("%v (piped) = %q, want []", args, out)
		}
	}
}

// A stored run shows as a table in a terminal.
func TestPostureRunsTableInTerminal(t *testing.T) {
	testenv.Isolate(t)
	db, err := store.Open(defaultDBPath("keeper-cli"))
	if err != nil {
		t.Fatal(err)
	}
	id, err := db.BeginPostureRun(context.Background(), 90)
	if err == nil {
		err = db.FinishPostureRun(context.Background(), id, "complete")
	}
	db.Close()
	if err != nil {
		t.Fatal(err)
	}
	var tty bytes.Buffer
	cmd := RootCmd()
	cmd.SetOut(terminalWriter{&tty})
	cmd.SetErr(&tty)
	cmd.SetArgs([]string{"posture", "runs"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	got := tty.String()
	if !strings.Contains(strings.ToUpper(got), "STATUS") || !strings.Contains(got, "complete") || strings.HasPrefix(strings.TrimSpace(got), "[") {
		t.Fatalf("terminal output:\n%s", got)
	}
}
