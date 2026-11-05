// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored: guards handfixes.json journal-credential-hardening.

package learn_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"crowdstrike-pp-cli/internal/credkit"
)

// journalHardeningSecret stands in for a credential an operator put where a
// session label or a flag name goes (low entropy on purpose).
const journalHardeningSecret = "jh-credential-aaaabbbbcccc"

// filesCarrying lists every file under root whose bytes contain needle.
func filesCarrying(t *testing.T, root, needle string) []string {
	t.Helper()
	var hits []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := os.ReadFile(p) // #nosec G304 -- temp test path
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
// the learn journal and teach.log, not held and then held by the shared
// credential kit, and fails if the credential reaches any file under the test
// home. Each run must journal one entry, so a run that writes nothing (or
// whose entry the kit withheld) cannot pass.
func TestJournalCredentialHardening(t *testing.T) {
	cases := []struct {
		name     string
		override bool
		args     []string
		needle   string // what must reach no file; the secret unless set
		noShape  bool   // nothing after "--" is a flag, so argv_shape stays empty
	}{
		{name: "learn session override", override: true, args: []string{"version"}},
		{name: "credential typed as a flag name", args: []string{"version", "--" + journalHardeningSecret}},
		{name: "credential typed as a flag name with a value", args: []string{"version", "--" + journalHardeningSecret + "=x"}},
		{name: "credential typed as a short flag", args: []string{"version", "-" + journalHardeningSecret}},
		{name: "credential after the end of flags", args: []string{"version", "--", "--" + journalHardeningSecret}, noShape: true},
		// Within 3 edits of the declared --compact, so Cobra suggests it: the
		// typed name is still never kept.
		{name: "credential one typo from a declared flag", args: []string{"version", "--compactQ7x"}, needle: "compactQ7x"},
		{name: "declared flag name after the end of flags", args: []string{"version", "--", "--json"}, noShape: true},
		{name: "teach with no query", args: []string{"teach", "--", "--" + journalHardeningSecret}},
		{name: "playbook amend with no query", args: []string{"playbook", "amend", "--", "--" + journalHardeningSecret}},
	}
	for _, held := range []bool{false, true} {
		if held {
			// A kit hold is process-wide, so the held pass runs second.
			credkit.Hold("minted:token", journalHardeningSecret)
		}
		for _, tc := range cases {
			name := tc.name
			if held {
				name += " (held)"
			}
			t.Run(name, func(t *testing.T) {
				home := withJournalHome(t)
				if tc.override {
					t.Setenv("CROWDSTRIKE_LEARN_SESSION", journalHardeningSecret)
				}
				_ = runCLI(t, tc.args...)
				entries := readAllJournalEntries(t)
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
					needle = journalHardeningSecret
				}
				if hits := filesCarrying(t, home, needle); len(hits) > 0 {
					t.Errorf("the credential reached %v", hits)
				}
			})
		}
	}
}
