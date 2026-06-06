// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Execution test: completeness audit runs against the real store schema and
// emits valid JSON (empty result on an empty mirror, not an error).

package cli

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
)

// runNovelCmd executes a hand-authored novel subcommand against a temp DB in
// JSON mode and returns its stdout. Fails the test on any execution error.
func runNovelCmd(t *testing.T, build func(*rootFlags) *cobra.Command, extraArgs ...string) []byte {
	t.Helper()
	flags := &rootFlags{asJSON: true}
	cmd := build(flags)
	tmpDB := filepath.Join(t.TempDir(), "audit-test.db")
	// stdout and stderr stay separate: sync-staleness hints write to stderr
	// by design and must not pollute the machine-readable stdout JSON.
	var out, errBuf bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errBuf)
	args := append([]string{"--db", tmpDB}, extraArgs...)
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error: %v\nstdout: %s\nstderr: %s", err, out.String(), errBuf.String())
	}
	return out.Bytes()
}

func TestNovelAuditCompletenessRuns(t *testing.T) {
	out := runNovelCmd(t, newNovelAuditCompletenessCmd)
	var rows []completenessRow
	if err := json.Unmarshal(out, &rows); err != nil {
		t.Fatalf("output is not a JSON array: %v\n%s", err, out)
	}
	if len(rows) != 0 {
		t.Errorf("expected empty result on empty mirror, got %d rows", len(rows))
	}
}

func TestNovelAuditCompletenessCrossTenantRuns(t *testing.T) {
	out := runNovelCmd(t, newNovelAuditCompletenessCmd, "--cross-tenant")
	var rows []completenessRow
	if err := json.Unmarshal(out, &rows); err != nil {
		t.Fatalf("cross-tenant output is not a JSON array: %v\n%s", err, out)
	}
}
