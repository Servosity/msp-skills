// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package learn_test

import (
	"strings"
	"testing"

	"action1-pp-cli/internal/learn"
)

// Codex r1 (action1 reprint on 4.34.2) class credential persisted in journal:
// the journal writer fills the session key after the entry passed the output
// chokepoint, so a session key equal to the bearer token was written verbatim.
// The override is journaled hashed, so the token never reaches disk. A session
// key that is no credential is still recorded (as its hash).
func TestJournalNeverWritesAHeldSessionKey(t *testing.T) {
	withJournalHome(t)
	const token = "JournalCredential-A7p9zK3m"
	t.Setenv("ACTION1_OAUTH2", token)
	t.Setenv("ACTION1_LEARN_SESSION", token)
	if err := runCLI(t, "version"); err != nil {
		t.Fatalf("version: %v", err)
	}
	if raw := journalRawBytes(t); strings.Contains(raw, token) {
		t.Fatalf("the journal holds the bearer token: %s", raw)
	}

	withJournalHome(t)
	t.Setenv("ACTION1_OAUTH2", token)
	t.Setenv("ACTION1_LEARN_SESSION", "session-abc123")
	if err := runCLI(t, "version"); err != nil {
		t.Fatalf("version: %v", err)
	}
	// The override is journaled hashed (journal-credential-hardening), so a key
	// that is no credential is recorded as its hash, never dropped.
	if entries := readAllJournalEntries(t); len(entries) != 1 || entries[0].SessionKey != learn.JournalSessionKey() || !strings.HasPrefix(entries[0].SessionKey, "o:") {
		t.Fatalf("a session key that is no credential is not journaled: %+v", entries)
	}
}
