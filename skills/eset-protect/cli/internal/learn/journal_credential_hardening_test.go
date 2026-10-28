// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored (not generator output); pinned by skills/eset-protect/handfixes.json
// under "journal-credential-hardening".

package learn_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// journalHardeningSecret stands in for a credential an operator put where a
// session label, a flag name or an operand goes.
const journalHardeningSecret = "Zq7f3a9c0e5b1d42-jh-credential"

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

// TestJournalCredentialHardening drives the paths a value the kit does not
// hold could take into the learn journal, and fails if it reaches any file
// under the test home: the LEARN_SESSION override and a value typed as a flag
// name (before and after "--"). A held value in any file is proven on the built
// binaries by the leak tests in internal/credwire.
func TestJournalCredentialHardening(t *testing.T) {
	run := func(name string, f func(t *testing.T)) {
		t.Run(name, func(t *testing.T) {
			home := withJournalHome(t)
			f(t)
			if hits := filesCarrying(t, home, journalHardeningSecret); len(hits) > 0 {
				t.Fatalf("the credential reached %v", hits)
			}
			if len(readAllJournalEntries(t)) == 0 {
				t.Fatalf("no journal entry was written, so the probe proved nothing")
			}
		})
	}
	run("session override", func(t *testing.T) {
		t.Setenv("ESET_PROTECT_LEARN_SESSION", journalHardeningSecret)
		_ = runCLI(t, "version")
	})
	run("credential as flag name", func(t *testing.T) {
		_ = runCLI(t, "version", "--"+journalHardeningSecret)
	})
	run("credential as flag name after end of flags", func(t *testing.T) {
		_ = runCLI(t, "version", "--", "--"+journalHardeningSecret)
	})
	run("credential as equals flag name", func(t *testing.T) {
		_ = runCLI(t, "version", "--"+journalHardeningSecret+"=x")
	})
}
