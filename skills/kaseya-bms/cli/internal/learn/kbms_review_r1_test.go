// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package learn_test

import (
	"strings"
	"testing"

	"kaseya-bms-pp-cli/internal/credkit"
	"kaseya-bms-pp-cli/internal/learn"
)

// Codex r1 (kbms reprint on 4.34.2) class credential persisted in journal:
// the journal writer fills the session key after the entry passed the output
// chokepoint, so a session key equal to a credential (here the
// KASEYA_BMS_PASSWORD the environment carries for the BMS login the mint
// exchanges) was written verbatim.
// The entry is checked as written: one that repeats a held value is not
// journaled. A session key that is no credential is still recorded.
func TestJournalNeverWritesAHeldSessionKey(t *testing.T) {
	withJournalHome(t)
	const token = "JournalCredential-A7p9zK3m"
	t.Setenv("KASEYA_BMS_USERNAME", "journal-api-user-5c2e")
	t.Setenv("KASEYA_BMS_PASSWORD", token)
	t.Setenv("KASEYA_BMS_TENANT", "journal-tenant")
	t.Setenv("KASEYA_BMS_LEARN_SESSION", token)
	if err := runCLI(t, "version"); err != nil {
		t.Fatalf("version: %v", err)
	}
	if raw := journalRawBytes(t); strings.Contains(raw, token) {
		t.Fatalf("the journal holds the login password: %s", raw)
	}

	withJournalHome(t)
	t.Setenv("KASEYA_BMS_USERNAME", "journal-api-user-5c2e")
	t.Setenv("KASEYA_BMS_PASSWORD", token)
	t.Setenv("KASEYA_BMS_TENANT", "journal-tenant")
	t.Setenv("KASEYA_BMS_LEARN_SESSION", "session-abc123")
	if err := runCLI(t, "version"); err != nil {
		t.Fatalf("version: %v", err)
	}
	if entries := readAllJournalEntries(t); len(entries) != 1 || entries[0].SessionKey != "session-abc123" {
		t.Fatalf("a session key that is no credential is journaled: %+v", entries)
	}
}

// kaseya-bms handfix journal-entry-withheld-as-written: the learn context a
// learn command stages is merged into the entry inside JournalInvocation, after
// any caller's chokepoint, so the entry is checked as it will be written.
func TestJournalNeverWritesAHeldLearnContext(t *testing.T) {
	withJournalHome(t)
	const held = "LearnContextCredential-9x8y7w"
	credkit.Hold("test:held", held)
	learn.SetJournalLearnContext("tickets for "+held, []string{held})
	learn.JournalInvocation(learn.JournalEntry{Cmd: []string{"learnings", "teach"}})
	if raw := journalRawBytes(t); strings.Contains(raw, held) {
		t.Fatalf("the journal holds a held value from the learn context: %s", raw)
	}
}
