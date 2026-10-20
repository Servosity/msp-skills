// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// HAND-AUTHORED.

package cli

import "testing"

// TestInvalidJSONFlagIsUsageError: a JSON flag that does not parse is a usage
// error (exit 2, as SKILL.md documents), on every command that takes one, and
// nothing is sent.
func TestInvalidJSONFlagIsUsageError(t *testing.T) {
	sweepIsolate(t)
	t.Setenv("SYNOLOGY_BASE_URL", "https://192.0.2.1:5001")
	for _, args := range [][]string{
		{"abb", "backup", "--task-ids", "not-json", "--dry-run"},
		{"abb", "cancel", "--task-ids", "not-json", "--dry-run"},
		{"user", "--additional", "not-json", "--dry-run"},
	} {
		_, stderr, code := sweepExecute(t, args...)
		if code != 2 {
			t.Fatalf("%v exited %d, want 2 (usage error); stderr: %s", args, code, stderr)
		}
	}
}
