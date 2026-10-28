// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): guards handfixes.json journal-credential-hardening.

package learn_test

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"domain-cert-tracker-pp-cli/internal/cliutil"
	"domain-cert-tracker-pp-cli/internal/cliutil/testenv"
	"domain-cert-tracker-pp-cli/internal/learn"
)

// journalHardeningSecret stands in for a credential an operator put where a
// session label, a flag name or an operand goes.
const journalHardeningSecret = "Zq7f3a9c0e5b1d42-jh-credential"

// hashedForTest is the journal's session hash (hashHarnessSession).
func hashedForTest(v string) string {
	sum := sha256.Sum256([]byte(v))
	return hex.EncodeToString(sum[:8])
}

// hardeningHome sandboxes every user directory the CLI resolves (config,
// data, state, cache) through the press sandbox, so the walk below sees every
// file a run writes, and clears every journal switch.
func hardeningHome(t *testing.T) string {
	t.Helper()
	home := testenv.Isolate(t, cliutil.ConfigDir, cliutil.DataDir, cliutil.StateDir, cliutil.CacheDir)
	t.Setenv("PRINTING_PRESS_VERIFY", "")
	t.Setenv("PRINTING_PRESS_DOGFOOD", "")
	t.Setenv("DOMAIN_CERT_TRACKER_NO_LEARN", "")
	t.Setenv("DOMAIN_CERT_TRACKER_LEARN_NO_CAPTURE", "")
	t.Setenv("DOMAIN_CERT_TRACKER_LEARN_SESSION", "")
	for _, name := range learn.JournalHarnessSessionEnvVars {
		t.Setenv(name, "")
	}
	return home
}

// filesCarrying lists every file under root whose bytes contain needle.
func filesCarrying(t *testing.T, root, needle string) []string {
	t.Helper()
	var hits []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := os.ReadFile(p) // #nosec G304 -- walked from the test's own temp home
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
// the learn journal as typed text the journal never keeps (a session label, a
// flag name, text after "--"), and fails if it reaches any file under the test
// home (the journal, teach.log, anything). Each run must journal one entry, so
// a run that writes nothing cannot pass. A credential the program holds is the
// credential kit's to withhold at the file sink; the binary-level leak tests in
// internal/credwire cover that.
func TestJournalCredentialHardening(t *testing.T) {
	cases := []struct {
		name     string
		override bool
		args     []string
		needle   string // what must reach no file; the secret unless set
		noShape  bool   // nothing after "--" is a flag, so argv_shape stays empty
		typo     bool   // Cobra suggested a declared flag for the typed name
	}{
		{name: "learn session override", override: true, args: []string{"version"}},
		{name: "credential typed as a flag name", args: []string{"version", "--" + journalHardeningSecret}},
		{name: "credential typed as a flag name with a value", args: []string{"version", "--" + journalHardeningSecret + "=x"}},
		{name: "credential typed as a short flag", args: []string{"version", "-" + journalHardeningSecret}},
		{name: "credential after the end of flags", args: []string{"version", "--", "--" + journalHardeningSecret}, noShape: true},
		// Within 3 edits of the declared --compact, so Cobra suggests it: the
		// typed name is still never kept.
		{name: "credential one typo from a declared flag", args: []string{"version", "--compactQ7x"}, needle: "compactQ7x", typo: true},
		{name: "declared flag name after the end of flags", args: []string{"version", "--", "--json"}, noShape: true},
		{name: "teach with no query", args: []string{"teach", "--", "--" + journalHardeningSecret}},
		{name: "playbook amend with no query", args: []string{"playbook", "amend", "--", "--" + journalHardeningSecret}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			home := hardeningHome(t)
			if tc.override {
				t.Setenv("DOMAIN_CERT_TRACKER_LEARN_SESSION", journalHardeningSecret)
			}
			_ = runCLI(t, tc.args...)
			entries := readAllJournalEntries(t)
			if len(entries) != 1 {
				t.Fatalf("want 1 journal entry, got %d (a run that journals nothing proves nothing)", len(entries))
			}
			e := entries[0]
			if tc.override {
				if want := "o:" + hashedForTest(journalHardeningSecret); e.SessionKey != want {
					t.Errorf("session_key = %q, want the hashed override %q", e.SessionKey, want)
				}
			}
			if tc.noShape && len(e.ArgvShape) != 0 {
				t.Errorf("argv_shape = %v, want empty: every token after -- is an operand", e.ArgvShape)
			}
			if tc.typo {
				if want := "--<undeclared>:" + hashedForTest("--compactQ7x"); e.FailedFlag != want {
					t.Errorf("failed_flag = %q, want the opaque identity %q", e.FailedFlag, want)
				}
				if e.SuggestedFlag != "--compact" {
					t.Errorf("suggested_flag = %q, want --compact", e.SuggestedFlag)
				}
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
