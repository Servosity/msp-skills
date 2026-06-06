// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Execution test: stale-passwords audit runs against the real store schema.

package cli

import (
	"encoding/json"
	"testing"
)

func TestNovelAuditStalePasswordsRuns(t *testing.T) {
	out := runNovelCmd(t, newNovelAuditStalePasswordsCmd, "--older-than", "180d")
	var rows []stalePasswordRow
	if err := json.Unmarshal(out, &rows); err != nil {
		t.Fatalf("output is not a JSON array: %v\n%s", err, out)
	}
	if len(rows) != 0 {
		t.Errorf("expected empty result on empty mirror, got %d", len(rows))
	}
}

func TestNovelAuditStalePasswordsBadDuration(t *testing.T) {
	flags := &rootFlags{asJSON: true}
	cmd := newNovelAuditStalePasswordsCmd(flags)
	cmd.SetArgs([]string{"--older-than", "nonsense"})
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	if err := cmd.Execute(); err == nil {
		t.Error("expected error for invalid --older-than, got nil")
	}
}
