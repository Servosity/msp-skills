// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package learn_test

import (
	"strings"
	"testing"
)

// Codex r1 (appdirect reprint on 4.34.2) class credential persisted in journal:
// the journal writer fills the session key after the entry passed the output
// chokepoint, so a session key equal to a credential (here the
// APPDIRECT_CLIENT_SECRET the environment carries) was written verbatim.
// The entry is checked as written: one that repeats a held value is not
// journaled. A session key that is no credential is still recorded.
func TestJournalNeverWritesAHeldSessionKey(t *testing.T) {
	withJournalHome(t)
	const token = "JournalCredential-A7p9zK3m"
	t.Setenv("APPDIRECT_CLIENT_ID", "journal-client-id-5c2e")
	t.Setenv("APPDIRECT_CLIENT_SECRET", token)
	t.Setenv("APPDIRECT_LEARN_SESSION", token)
	if err := runCLI(t, "version"); err != nil {
		t.Fatalf("version: %v", err)
	}
	if raw := journalRawBytes(t); strings.Contains(raw, token) {
		t.Fatalf("the journal holds the client secret: %s", raw)
	}

	withJournalHome(t)
	t.Setenv("APPDIRECT_CLIENT_ID", "journal-client-id-5c2e")
	t.Setenv("APPDIRECT_CLIENT_SECRET", token)
	t.Setenv("APPDIRECT_LEARN_SESSION", "session-abc123")
	if err := runCLI(t, "version"); err != nil {
		t.Fatalf("version: %v", err)
	}
	if entries := readAllJournalEntries(t); len(entries) != 1 || entries[0].SessionKey != "session-abc123" {
		t.Fatalf("a session key that is no credential is journaled: %+v", entries)
	}
}
