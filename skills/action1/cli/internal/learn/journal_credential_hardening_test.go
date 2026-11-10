// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored: guards handfixes.json journal-credential-hardening
// The held pass holds through the shared credential kit (handfixes.json credkit-wiring).

package learn_test

import (
	"action1-pp-cli/internal/credkit"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// journalHardeningSecret stands in for a credential an operator put where a
// session label or a flag name goes. No 8-character piece of it occurs in a
// test's temp path (TestJournalCredentialHardening...), which the kit would
// then withhold from every file and database path.
const journalHardeningSecret = "jsec-wwqmzkzzqqkzqzmqzqmm"

// journalHardeningName stands in for a credential put where the program stores
// a name by design (a profile, a query, feedback). Lowercase letters and
// digits only, so normalization cannot split it into pieces a search misses.
const journalHardeningName = "zq7f3a9c0e5b1d42kx8w4m2p"

// journalHardeningProbe is a value that is not held, so it must be stored: it
// proves each operator-named sink is reached.
const journalHardeningProbe = "jhprobe7kq2xwzm"

// filesCarrying lists every file under root whose bytes contain needle, in any
// case (a sink may store a lowercased form).
func filesCarrying(t *testing.T, root, needle string) []string {
	t.Helper()
	needle = strings.ToLower(needle)
	var hits []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if strings.Contains(strings.ToLower(string(b)), needle) {
			hits = append(hits, p)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	return hits
}

func withValue(args []string, v string) []string {
	out := make([]string, len(args))
	for i, a := range args {
		out[i] = strings.ReplaceAll(a, "{v}", v)
	}
	return out
}

// journalHardeningHeldEnv marks the child process that runs the held pass.
const journalHardeningHeldEnv = "JOURNAL_HARDENING_HELD_PASS"

// TestJournalCredentialHardening drives each path a credential could take into
// the learn journal, not held and then held by the kit, and fails if the
// credential reaches any file under the test home (the journal, the learn
// store, anything). Each run must journal one entry, so a run that writes
// nothing cannot pass. In the held pass it also drives every sink that stores
// operator-named text by design: a probe must land in a file (the sink was
// reached), then a held credential in the same place must reach no file.
//
// The held pass runs in a child of this test binary: a held value and a
// refused write's one-time warning are both process-wide, and neither may
// reach another test in this package.
func TestJournalCredentialHardening(t *testing.T) {
	held := os.Getenv(journalHardeningHeldEnv) == "1"
	if held {
		credkit.Hold("test:journal", journalHardeningSecret, journalHardeningName)
	}
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
	// "{v}" is the probe, then the held credential.
	operands := []struct {
		name string
		args []string
	}{
		{name: "profile name", args: []string{"profile", "save", "--json", "--", "{v}"}},
		{name: "profile description", args: []string{"profile", "save", "jh-profile", "--json", "--description", "{v}"}},
		{name: "recall query", args: []string{"recall", "--", "{v}"}},
		{name: "teach query", args: []string{"teach", "--query", "{v}", "--resource-type", "items", "--resource", "jh-resource"}},
		{name: "teach resource", args: []string{"teach", "--query", "find items", "--resource-type", "items", "--resource", "{v}"}},
		{name: "teach notes", args: []string{"teach", "--query", "find items", "--resource-type", "items", "--resource", "jh-resource", "--notes", "{v}"}},
		{name: "teach pattern", args: []string{"teach-pattern", "--query-template", "{v} in {entity}", "--resource-template", "GROUP-{entity:category}", "--resource-type", "items", "--entity-kind", "category"}},
		{name: "entity alias", args: []string{"teach-lookup", "--kind", "category", "--canonical", "jh canonical", "--value", "{v}"}},
		{name: "feedback text", args: []string{"feedback", "{v}"}},
	}
	for _, tc := range cases {
		name := tc.name
		if held {
			name += " (held)"
		}
		t.Run(name, func(t *testing.T) {
			home := withJournalHome(t)
			if tc.override {
				t.Setenv("ACTION1_LEARN_SESSION", journalHardeningSecret)
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
	if !held {
		// A value the program does not hold is a name, stored by design.
		child := exec.Command(os.Args[0], "-test.run=^TestJournalCredentialHardening$", "-test.v") // #nosec G204 -- this test binary
		child.Env = append(os.Environ(), journalHardeningHeldEnv+"=1")
		out, err := child.CombinedOutput()
		if err != nil || !strings.Contains(string(out), "(held)") {
			t.Fatalf("held pass: %v\n%s", err, out)
		}
		// The held pass runs in the child by construction (exec of this test binary).
		return
	}
	for _, op := range operands {
		t.Run(op.name+" (held)", func(t *testing.T) {
			home := withJournalHome(t)
			_ = runCLI(t, withValue(op.args, journalHardeningProbe)...)
			if len(filesCarrying(t, home, journalHardeningProbe)) == 0 {
				t.Fatalf("the probe reached no file, so this run proves nothing about the sink")
			}
			home = withJournalHome(t)
			_ = runCLI(t, withValue(op.args, journalHardeningName)...)
			if hits := synccpuNotStore(filesCarrying(t, home, journalHardeningName)); len(hits) > 0 {
				t.Errorf("the held credential reached %v", hits)
			}
		})
	}
}

// TestJournalHardeningStartupOverride starts this test binary with the
// override already in its environment, so code that reads it at package init,
// before any t.Setenv could, is exercised too: nothing it writes may carry it.
func TestJournalHardeningStartupOverride(t *testing.T) {
	home := withJournalHome(t)
	child := exec.Command(os.Args[0], "-test.run=^$") // #nosec G204 -- this test binary
	child.Env = append(os.Environ(), "ACTION1_LEARN_SESSION="+journalHardeningSecret)
	if out, err := child.CombinedOutput(); err != nil {
		t.Fatalf("child process: %v\n%s", err, out)
	}
	if hits := filesCarrying(t, home, journalHardeningSecret); len(hits) > 0 {
		t.Errorf("the override read at startup reached %v", hits)
	}
}

// TestJournalFailedFlagIdentity: two typos Cobra corrects to one flag keep two
// opaque identities, so rejecting one correction never suppresses the other.
func TestJournalFailedFlagIdentity(t *testing.T) {
	withJournalHome(t)
	_ = runCLI(t, "version", "--compac")
	_ = runCLI(t, "version", "--cmpact")
	entries := readAllJournalEntries(t)
	if len(entries) != 2 {
		t.Fatalf("want 2 journal entries, got %d", len(entries))
	}
	a, b := entries[0].FailedFlag, entries[1].FailedFlag
	if !strings.HasPrefix(a, "--<undeclared>:") || !strings.HasPrefix(b, "--<undeclared>:") || a == b {
		t.Errorf("failed flags %q and %q: want two distinct opaque identities", a, b)
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
