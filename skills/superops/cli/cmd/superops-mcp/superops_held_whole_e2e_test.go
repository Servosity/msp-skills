// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): the third next-batch review's class D inputs,
// driven through the real binaries (credkit-wiring): an operator file is held
// whole where it is read, every authority form is read, a metadata write passes
// the kit.

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestJournalOffsetPassesTheKit(t *testing.T) {
	t.Parallel()
	cliBin := buildHolderBinary(t, "superops-cli", "../superops-cli")
	run := func(t *testing.T, token string) string {
		sandbox := t.TempDir()
		env := holderEnv(sandbox, map[string]string{
			"SUPEROPS_API_TOKEN": token, "SUPEROPS_BASE_URL": "http://127.0.0.1:9", "SUPEROPS_SUBDOMAIN": "acme-holder",
			"SUPEROPS_STATE_DIR": "{sandbox}/st", "PRINTING_PRESS_DOGFOOD": "",
		})
		dir := filepath.Join(sandbox, "st", "learn")
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		segment := filepath.Join(dir, "journal-"+time.Now().UTC().Format("20060102")+".jsonl")
		if err := os.WriteFile(segment, []byte(`{"cmd":["version"],"exit_code":0}`+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		_ = runHolderCLI(t, cliBin, env, "", "version")
		b, _ := os.ReadFile(filepath.Join(dir, "journal.offset"))
		return string(b)
	}
	// Control: with a token that repeats nothing the offset names its segment,
	// so the run below does reach the write.
	if got := run(t, "Cobalt-OpalSecret4340"); !strings.Contains(got, "journal-") {
		t.Fatalf("the control run wrote no offset naming its segment: %q", got)
	}
	if got := run(t, "Cobaltjournal-OpalSecret"); strings.Contains(got, "journal-") {
		t.Errorf("journal.offset carries a held 8-character piece: %q", got)
	}
}
