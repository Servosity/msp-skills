// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package store

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Class store-split-bindings (kit adoption C1): a held credential split
// across two bound values of one statement, each half of 8 or more
// characters, is one unit (the statement text and each bound value on a line
// of its own). The store never keeps either half, whichever writer carries
// it: a raw INSERT and a learning whose Query and Notes each carry a half.
//
// The held value stays held for the rest of the process (the kit has no
// release), and every later statement in the package would then pay the
// kit's full scan (the package ran past go test's ten-minute limit on
// Linux), so the check runs in a child test process of its own.
func TestCredentialSplitAcrossBindingsNeverPersists(t *testing.T) {
	if os.Getenv("KBMS_STORE_SPLIT_CHILD") != "1" {
		cmd := exec.Command(os.Args[0], "-test.run=^TestCredentialSplitAcrossBindingsNeverPersists$", "-test.count=1", "-test.v")
		cmd.Env = append(os.Environ(), "KBMS_STORE_SPLIT_CHILD=1")
		out, err := cmd.CombinedOutput()
		if err != nil || !strings.Contains(string(out), "--- PASS: TestCredentialSplitAcrossBindingsNeverPersists") {
			t.Fatalf("the split-bindings check failed in its own process (err=%v):\n%s", err, out)
		}
		return
	}
	const secret = "splitHeldSecret-" + "4b8e1c7a9d2f6e30"
	holdForSplitTest(secret)
	left, right := secret[:len(secret)/2], secret[len(secret)/2:]
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := s.db.ExecContext(ctx, `CREATE TABLE kbms_split (a TEXT, b TEXT)`); err != nil {
		t.Fatal(err)
	}
	// Refused or withheld are both fine; keeping either half is not.
	_, _ = s.db.ExecContext(ctx, `INSERT INTO kbms_split (a, b) VALUES (?, ?)`, "left "+left, right+" right")
	_, _, _ = s.UpsertLearning(ctx, UpsertLearningInput{Query: "find " + left, ResourceType: "servicedesk", ResourceID: "split-1", Notes: right + " rotated"})
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		for _, half := range []string{left, right} {
			if bytes.Contains(b, []byte(half)) {
				t.Errorf("%s keeps the half %q of a held credential split across two bindings", e.Name(), half)
			}
		}
	}
}
