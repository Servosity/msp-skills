// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): next-batch Codex round 1 on the zammad
// 4.34.0 publish (2026-10-27). Pinned by this connector's handfixes.json
// under the "credkit-wiring" entry.

package learn_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"zammad-pp-cli/internal/cliutil"
	"zammad-pp-cli/internal/credkit"
	"zammad-pp-cli/internal/learn"
)

// TestNBLearnRecordsAreCheckedWhole: the journal checked failed_flag, but
// argv_shape recorded the same flag name raw, so a token typed as a flag
// was written to disk (Codex nb-r1 [P1]). The class: every learn record is
// checked whole at its one write point, the journal and the teach log both.
func TestNBLearnRecordsAreCheckedWhole(t *testing.T) {
	if !kitInChild(t) {
		return
	}
	withJournalHome(t)
	token := "nbj-" + strings.Repeat("Jr4n", 12)
	t.Setenv("ZAMMAD_API_TOKEN", token)
	_ = runCLI(t, "tickets", "get", "42", "--"+token)

	teach := "nbt-" + strings.Repeat("Tl5n", 12)
	credkit.Hold("minted:test", teach)
	// Refused (the adopt pass: operator text repeating a held value is not saved) or withheld; never written.
	if err := learn.AppendTeachLogWarning("teach", "open tickets", learn.Warning{Code: "no_entity_overlap", Detail: "matched " + teach}); err != nil && !errors.Is(err, credkit.ErrWithheld) {
		t.Fatalf("teach log write: %v", err)
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
		for _, secret := range []string{token, teach} {
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
