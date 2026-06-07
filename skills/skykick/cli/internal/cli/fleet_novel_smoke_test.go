// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored shared harness for novel-command wiring tests.
package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// runNovelDry executes a novel command's RunE with --dry-run semantics and
// returns its stdout. Asserts the dry-run path exits clean.
func runNovelDry(t *testing.T, cmd *cobra.Command, args []string) string {
	t.Helper()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.RunE(cmd, args); err != nil {
		t.Fatalf("dry-run RunE(%v) error: %v", args, err)
	}
	return out.String()
}

// assertWould asserts dry-run output announces the would-do action.
func assertWould(t *testing.T, output string) {
	t.Helper()
	if !strings.Contains(output, "would") {
		t.Errorf("dry-run output should describe the would-do action, got: %q", output)
	}
}

// assertFlags asserts every named flag is declared on the command.
func assertFlags(t *testing.T, cmd *cobra.Command, names ...string) {
	t.Helper()
	for _, name := range names {
		if cmd.Flags().Lookup(name) == nil {
			t.Errorf("flag --%s not declared on %s", name, cmd.Use)
		}
	}
}
