// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored: guards handfixes.json journal-credential-hardening.

package learn_test

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"hubspot-pp-cli/internal/credkit"
	"hubspot-pp-cli/internal/learn"
)

// journalHardeningHeld stands in for a credential an operator put where a
// session label, a flag name, a name or an operand goes.
const journalHardeningHeld = "Zq7f3a9c0e5b1d42Jh8Kw2Xv"

// filesCarrying lists every file under root whose bytes contain needle.
func filesCarrying(t *testing.T, root, needle string) []string {
	t.Helper()
	var hits []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if strings.Contains(string(b), needle) {
			hits = append(hits, p)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	return hits
}

// TestJournalCredentialHardening drives each path a credential could take into
// the learn journal or any other file the program persists, not held and then
// held, and fails if the credential reaches any file under the test home (the
// journal, the learn store, teach.log, profiles, the feedback ledger,
// anything) or any request to the feedback endpoint. Each run must journal one
// entry, so a run that writes nothing cannot pass. heldOnly cases persist
// what the operator typed by design until the value is held.
func TestJournalCredentialHardening(t *testing.T) {
	if runInChild(t) {
		return
	}
	var posted []string
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b := make([]byte, 1<<16)
		n, _ := r.Body.Read(b)
		posted = append(posted, r.URL.String()+" "+string(b[:n]))
	}))
	defer endpoint.Close()
	cases := []struct {
		name     string
		override bool
		args     []string
		needle   string // what must reach no file; the secret unless set
		noShape  bool   // nothing after "--" is a flag, so argv_shape stays empty
		heldOnly bool
	}{
		{name: "learn session override", override: true, args: []string{"version"}},
		{name: "credential typed as a flag name", args: []string{"version", "--" + journalHardeningHeld}},
		{name: "credential typed as a flag name with a value", args: []string{"version", "--" + journalHardeningHeld + "=x"}},
		{name: "credential typed as a short flag", args: []string{"version", "-" + journalHardeningHeld}},
		{name: "credential after the end of flags", args: []string{"version", "--", "--" + journalHardeningHeld}, noShape: true},
		// Within 3 edits of the declared --compact, so Cobra suggests it: the
		// typed name is still never kept.
		{name: "credential one typo from a declared flag", args: []string{"version", "--compactQ7x"}, needle: "compactQ7x"},
		{name: "declared flag name after the end of flags", args: []string{"version", "--", "--json"}, noShape: true},
		{name: "teach with no query", args: []string{"teach", "--", "--" + journalHardeningHeld}},
		{name: "playbook amend with no query", args: []string{"playbook", "amend", "--", "--" + journalHardeningHeld}},
		// (c) a held credential typed as a name, label or operand is refused, not written.
		{name: "teach query", args: []string{"teach", "--query", "deals for " + journalHardeningHeld, "--resource", "123", "--resource-type", "deals"}, heldOnly: true},
		{name: "teach resource", args: []string{"teach", "--query", "open deals", "--resource", journalHardeningHeld, "--resource-type", "deals"}, heldOnly: true},
		{name: "recall query", args: []string{"recall", journalHardeningHeld + " deals"}, heldOnly: true},
		{name: "teach-lookup value", args: []string{"teach-lookup", "--kind", "team", "--canonical", "Sales", "--value", journalHardeningHeld}, heldOnly: true},
		{name: "profile name", args: []string{"profile", "save", journalHardeningHeld, "--json"}, heldOnly: true},
		{name: "feedback text", args: []string{"feedback", "it broke on " + journalHardeningHeld, "--send"}, heldOnly: true},
	}
	for _, held := range []bool{false, true} {
		if held {
			// credkit.Hold is process-global, so the held pass runs second.
			credkit.Hold("config:test", journalHardeningHeld)
		}
		for _, tc := range cases {
			if tc.heldOnly && !held {
				continue
			}
			name := tc.name
			if held {
				name += " (held)"
			}
			t.Run(name, func(t *testing.T) {
				home := withJournalHome(t)
				t.Setenv("HUBSPOT_FEEDBACK_ENDPOINT", endpoint.URL)
				posted = nil
				if tc.override {
					t.Setenv("HUBSPOT_LEARN_SESSION", journalHardeningHeld)
				}
				_ = runCLI(t, tc.args...)
				entries := readAllJournalEntries(t)
				// The kit withholds the whole record when a field carries a held value.
				if tc.heldOnly && len(entries) == 0 {
					if hits := filesCarrying(t, home, journalHardeningHeld); len(hits) > 0 {
						t.Errorf("the credential reached %v", hits)
					}
					return
				}
				if len(entries) != 1 {
					t.Fatalf("want 1 journal entry, got %d (a run that journals nothing proves nothing)", len(entries))
				}
				if tc.override && !strings.HasPrefix(entries[0].SessionKey, "o:") {
					t.Errorf("session_key = %q, want the hashed override (o:)", entries[0].SessionKey)
				}
				if tc.noShape && len(entries[0].ArgvShape) != 0 {
					t.Errorf("argv_shape = %v, want empty: every token after -- is an operand", entries[0].ArgvShape)
				}
				needle := tc.needle
				if needle == "" {
					needle = journalHardeningHeld
				}
				hits := filesCarrying(t, home, needle)
				if held {
					hits = synccpuNotStore(hits) // synccpu: the local store is not checked for a held value
				}
				if len(hits) > 0 {
					t.Errorf("the credential reached %v", hits)
				}
				for _, p := range posted {
					if strings.Contains(p, needle) {
						t.Errorf("the credential was sent to another host: %s", p)
					}
				}
			})
		}
	}
}

// The backstop, checked once every field is filled: an entry that still
// carries a held credential in any field is refused, not written.
func TestJournalRefusesAnEntryCarryingAHeldCredential(t *testing.T) {
	if runInChild(t) {
		return
	}
	home := withJournalHome(t)
	const v = "N4jZ8qK2vX7wT1pL9sB3"
	credkit.Hold("config:test", v)
	// The kit replaces the record with its notice, so no entry reads back and
	// the value is in no file (handfixes.json journal-credential-hardening).
	if err := learn.AppendJournalEntry(learn.JournalEntry{Cmd: []string{"recall"}, QueryFamily: "deals for " + v}); err != nil {
		t.Errorf("append: %v", err)
	}
	if n := len(readAllJournalEntries(t)); n != 0 {
		t.Errorf("an entry carrying a held credential was written (%d read back)", n)
	}
	if hits := filesCarrying(t, home, v); len(hits) > 0 {
		t.Errorf("the credential reached %v", hits)
	}
	if err := learn.AppendJournalEntry(learn.JournalEntry{Cmd: []string{"recall"}, QueryFamily: "open deals"}); err != nil || len(readAllJournalEntries(t)) != 1 {
		t.Errorf("a clean entry was not written: %v", err)
	}
}

// synccpuNotStore drops the SQLite store files: the kit no longer checks what
// the local store holds (credential leak contract section 5, owner GO
// 2026-11-09); every other file the run wrote is still checked.
func synccpuNotStore(paths []string) []string {
	var out []string
	for _, p := range paths {
		if !strings.HasSuffix(p, ".db") && !strings.Contains(p, ".db-") {
			out = append(out, p)
		}
	}
	return out
}
