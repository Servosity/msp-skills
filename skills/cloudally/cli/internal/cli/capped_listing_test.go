// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"cloudally-pp-cli/internal/store"
)

// Codex round 1: a capped listing dropped rows with no notice, so 501
// failing mailboxes read as a complete 500-row result. Every capped listing
// in the new commands says, on stderr, how many it showed of how many and
// which flag lifts the cap; stdout keeps its shape.
func wantCapNotice(t *testing.T, what, stderr string, shown, total int) {
	t.Helper()
	want := fmt.Sprintf("showing %d of %d", shown, total)
	if !strings.Contains(stderr, want) || !strings.Contains(stderr, "--limit 0") {
		t.Fatalf("%s: stderr must say %q and name --limit 0, got %q", what, want, stderr)
	}
}

func TestBackupHealthSaysWhenItsLimitCutsRows(t *testing.T) {
	isolateAuthState(t)
	resetStoreScope(t)
	_, _ = runHealth("--json")
	seedHealthRows(t,
		`{"userId":"u1","taskId":"t1","source":"MS365","entityName":"a@contoso.com","lastBackupDate":"2020-01-01T00:00:00Z"}`,
		`{"userId":"u1","taskId":"t1","source":"MS365","entityName":"b@contoso.com","lastBackupDate":"2020-01-01T00:00:00Z"}`,
		`{"userId":"u2","taskId":"t2","source":"MS365","entityName":"c@fabrikam.com","lastBackupDate":"2020-01-01T00:00:00Z"}`,
	)
	_, stderr, err := runAuthCmd(t, "", "backup", "health", "--limit", "2", "--json")
	if err != nil {
		t.Fatal(err)
	}
	wantCapNotice(t, "backup health", stderr, 2, 3)
	_, stderr, err = runAuthCmd(t, "", "backup", "health", "--group-by", "account", "--limit", "1", "--json")
	if err != nil {
		t.Fatal(err)
	}
	wantCapNotice(t, "backup health --group-by account", stderr, 1, 2)
	if _, stderr, _ := runAuthCmd(t, "", "backup", "health", "--json"); strings.Contains(stderr, "showing") {
		t.Fatalf("an uncut listing must not claim a cap: %q", stderr)
	}
}

func TestRestoreHistorySaysWhenItsLimitCutsRows(t *testing.T) {
	isolateAuthState(t)
	configureDefaultDBScope("")
	st, err := store.OpenWithContext(context.Background(), defaultDBPath(restoreCLIName))
	if err != nil {
		t.Fatal(err)
	}
	db := st.DB()
	for i := 0; i < 3; i++ {
		b := restoreBatch{BatchID: fmt.Sprintf("20261001-12000%d-cap%d", i, i), BatchKey: fmt.Sprintf("k%d", i), ResumeKey: fmt.Sprintf("r%d", i),
			CreatedAt: time.Now().UTC().Add(time.Duration(i) * time.Second), Scope: scopeDistributor, TaskID: "64b0e9", Source: "MS365",
			Dates: []string{"2026-10-01"}, Options: map[string]any{}}
		if err := ledgerInsertBatch(db, b); err != nil {
			t.Fatal(err)
		}
		for j := 0; j < 3; j++ {
			if err := ledgerInsertJob(db, restoreJob{BatchID: b.BatchID, Entity: fmt.Sprintf("u%d@contoso.com", j), Requested: "2026-10-01",
				ResolvedRaw: fakeBackupRaw, Resolved: "x"}); err != nil {
				t.Fatal(err)
			}
		}
	}
	st.Close()
	_, stderr, err := runAuthCmd(t, "", "restore", "history", "--limit", "2", "--json")
	if err != nil {
		t.Fatal(err)
	}
	wantCapNotice(t, "restore history", stderr, 2, 3)
	_, stderr, err = runAuthCmd(t, "", "restore", "history", "--batch", "20261001-120000-cap0", "--limit", "2", "--json")
	if err != nil {
		t.Fatal(err)
	}
	wantCapNotice(t, "restore history --batch", stderr, 2, 3)
}
