// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): credkit-wiring learn journal.

package learn_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"superops-pp-cli/internal/credkit"
	"superops-pp-cli/internal/learn"
)

// The round-2 input: a held value as a flag name or failed flag is never
// journaled; a clean entry still is.
func TestJournalNeverKeepsAHeldValue(t *testing.T) {
	if !heldChild(t) {
		return
	}
	dir := withJournalHome(t)
	const held = "JournalHeldTok4488"
	credkit.Hold("minted:test", held)
	learn.JournalInvocation(learn.JournalEntry{Cmd: []string{"superops-cli"}, ArgvShape: map[string]string{held: "bool"}, FailedFlag: held, ExitCode: 1})
	learn.JournalInvocation(learn.JournalEntry{Cmd: []string{"superops-cli", "doctor"}, ExitCode: 0})
	var all strings.Builder
	_ = filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			b, _ := os.ReadFile(p)
			all.Write(b)
		}
		return nil
	})
	if strings.Contains(all.String(), held) || !strings.Contains(all.String(), `"doctor"`) {
		t.Errorf("journal files:\n%s", all.String())
	}
}
