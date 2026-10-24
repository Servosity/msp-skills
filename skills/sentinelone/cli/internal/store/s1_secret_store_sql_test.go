// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): the old-store scrub (s1_secret_store.go)
// applies the shared field rule itself, not a SQL copy of it.

package store

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"sentinelone-pp-cli/internal/cliutil"
)

// TestScrubFollowsTheSharedFieldRule pins the old-store scrub to the shared
// field rule, so a part added to the rule is never silently skipped by the
// scrub. The scrub has no SQL prefilter to drift from the rule: every TEXT
// and BLOB value goes through cliutil.RedactSecretFields. One key per rule
// part, in a typed table the old hand-picked list never named, plus two keys
// a SQL prefilter could not see (a JSON escape, and a key that only Go's
// Unicode case folding turns into "token": a KELVIN SIGN for the k), must all
// be gone from the file after an older build's store is opened.
func TestScrubFollowsTheSharedFieldRule(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "data.db")
	var stmts []olderBuildStmt
	var sentinels, ordinaries []string
	for i, part := range cliutil.SecretFieldParts() {
		id := fmt.Sprintf("n%02d", i)
		sentinel := fmt.Sprintf("RULEPART%02dSENTINEL", i)
		ordinary := fmt.Sprintf("ORDINARYNOTE%02d", i)
		data := fmt.Sprintf(`{"id":%q,"keep":%q,"x%sField":%q}`, id, ordinary, part, sentinel)
		stmts = append(stmts, olderBuildStmt{`INSERT INTO notes (id, threats_id, data) VALUES (?, 't1', ?)`, []any{id, data}})
		sentinels = append(sentinels, sentinel)
		ordinaries = append(ordinaries, ordinary)
	}
	backslash := string(rune(92))
	escapedKey := backslash + "u0070assword" // a JSON escape that spells "password"
	kelvinKey := "TO" + string(rune(0x212A)) + "EN"
	stmts = append(stmts,
		olderBuildStmt{`INSERT INTO resources (id, resource_type, data) VALUES ('e1', 'agents', ?)`,
			[]any{`{"id":"e1","keep":"ORDINARYESCAPED","` + escapedKey + `":"ESCAPEDPWSENTINEL"}`}},
		olderBuildStmt{`INSERT INTO resources (id, resource_type, data) VALUES ('k1', 'sites', ?)`,
			[]any{`{"id":"k1","keep":"ORDINARYKELVIN","` + kelvinKey + `":"KELVINTOKENSENTINEL"}`}},
	)
	sentinels = append(sentinels, "ESCAPEDPWSENTINEL", "KELVINTOKENSENTINEL")
	ordinaries = append(ordinaries, "ORDINARYESCAPED", "ORDINARYKELVIN")
	writeOlderBuildStore(t, dbPath, stmts...)

	s, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if leaks := secretStoreFileLeaks(t, dir, sentinels...); len(leaks) > 0 {
		t.Fatalf("the scrub missed keys the field rule names:\n%s", strings.Join(leaks, "\n"))
	}
	for _, o := range ordinaries {
		if len(secretStoreFileLeaks(t, dir, o)) == 0 {
			t.Errorf("ordinary value %q is no longer in the store", o)
		}
	}
}
