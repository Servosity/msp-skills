// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored: guards handfixes.json journal-credential-hardening once root.go
// passes the invoked command's declared-flag lookup (proposed, H3).

package learn_test

import "testing"

// TestJournalKeepsDeclaredFlagsOnly runs the real command tree: a declared flag
// is kept by name, an undeclared one only as the placeholder, and a teach with no
// query writes no raw argument to teach.log.
func TestJournalKeepsDeclaredFlagsOnly(t *testing.T) {
	t.Run("declared and undeclared", func(t *testing.T) {
		home := journalHardeningHome(t)
		_ = runCLI(t, "version", "--json", "--"+journalHardeningSecret)
		entries := readAllJournalEntries(t)
		if len(entries) != 1 {
			t.Fatalf("want 1 entry, got %d", len(entries))
		}
		got := entries[0].ArgvShape
		if len(got) != 2 || got["json"] != "bool" || got["<undeclared>"] != "redacted" {
			t.Errorf("argv_shape = %v, want json:bool and <undeclared>:redacted", got)
		}
		if hits := credentialPieces(t, home, journalHardeningSecret); len(hits) > 0 {
			t.Errorf("the credential reached a file: %v", hits)
		}
	})
	t.Run("typo pairs with the placeholder", func(t *testing.T) {
		journalHardeningHome(t)
		_ = runCLI(t, "version", "--jso")
		_ = runCLI(t, "version", "--json")
		entries := readAllJournalEntries(t)
		if len(entries) != 2 || entries[0].FailedFlag != "--<undeclared>" || entries[0].SuggestedFlag != "--json" || entries[1].ArgvShape["json"] != "bool" {
			t.Errorf("entries = %+v, want a placeholder failure then a declared --json success", entries)
		}
	})
	for _, args := range [][]string{{"teach", "--", "--" + journalHardeningSecret}, {"playbook", "amend", "--", "--" + journalHardeningSecret}} {
		t.Run(args[0]+" with no query", func(t *testing.T) {
			home := journalHardeningHome(t)
			_ = runCLI(t, args...)
			if hits := credentialPieces(t, home, journalHardeningSecret); len(hits) > 0 {
				t.Errorf("the credential reached a file: %v", hits)
			}
		})
	}
}
