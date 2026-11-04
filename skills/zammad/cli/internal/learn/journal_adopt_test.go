// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): the adopt pass on the zammad 4.34.0 publish
// (2026-10-28). Pinned by this connector's handfixes.json under the
// "credkit-wiring" entry.

package learn_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"zammad-pp-cli/internal/cliutil"
	"zammad-pp-cli/internal/credkit"
	"zammad-pp-cli/internal/learn"
)

// TestAdoptJournalCheckedAtTheWritePoint: the record check ran in
// JournalInvocation, and AppendJournalEntry then filled session_key with
// the raw ZAMMAD_LEARN_SESSION and wrote it unchecked (Codex nb-r2 [P1],
// repeating nb-r1-4). The class: a journal record is checked at the write
// point, after every field is filled, whoever calls AppendJournalEntry.
func TestAdoptJournalCheckedAtTheWritePoint(t *testing.T) {
	if !kitInChild(t) {
		return
	}
	withJournalHome(t)
	token := "adj-" + strings.Repeat("Jw6p", 12)
	t.Setenv("ZAMMAD_API_TOKEN", token)
	t.Setenv("ZAMMAD_LEARN_SESSION", token)
	_ = runCLI(t, "version")

	direct := "adk-" + strings.Repeat("Kw7p", 12)
	credkit.Hold("minted:test", direct)
	t.Setenv("ZAMMAD_LEARN_SESSION", direct)
	if err := learn.AppendJournalEntry(learn.JournalEntry{Cmd: []string{"version"}}); err != nil {
		t.Fatalf("journal write: %v", err)
	}
	if err := learn.AppendJournalEntry(learn.JournalEntry{Cmd: []string{"tickets", "get", direct}}); err != nil {
		t.Fatalf("journal write: %v", err)
	}

	state, err := cliutil.StateDir()
	if err != nil {
		t.Fatal(err)
	}
	wrote := 0
	_ = filepath.Walk(state, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		data, _ := os.ReadFile(p)
		wrote++
		for _, secret := range []string{token, direct} {
			if strings.Contains(string(data), secret) {
				t.Errorf("%s holds a held value: %s", p, data)
			}
		}
		return nil
	})
	if wrote == 0 {
		t.Fatal("nothing was written under the state dir: the test is vacuous")
	}
}

// kitInChild reports whether the calling test is in a child of this test
// binary, starting that child when it is not: the kit's held set lives for
// the process and has no reset, so a test that holds a value starts with
// `if !kitInChild(t) { return }` and its held set dies with the child.
func kitInChild(t *testing.T) bool {
	t.Helper()
	if os.Getenv("ZAMMAD_KIT_CHILD") != "" {
		return true
	}
	child := exec.Command(os.Args[0], "-test.run=^"+t.Name()+"$")
	child.Env = append(os.Environ(), "ZAMMAD_KIT_CHILD=1")
	if out, err := child.CombinedOutput(); err != nil {
		t.Fatalf("child: %v\n%s", err, out)
	}
	return false
}
