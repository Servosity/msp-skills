// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
package cli

import (
	"fmt"
	"testing"
	"time"
)

func seedStaleFixture(t *testing.T) string {
	db, path := newTestStore(t)
	fresh := time.Now().UTC().Add(-2 * time.Hour).Format(time.RFC3339)
	old := time.Now().UTC().Add(-100 * time.Hour).Format(time.RFC3339)
	seed(t, db, "resources", "res1", `{"id":"res1","tenant_id":"ten1","name":"Fresh","kind":"user"}`)
	seed(t, db, "resources", "res2", `{"id":"res2","tenant_id":"ten1","name":"Stale","kind":"user"}`)
	seed(t, db, "resources", "res3", `{"id":"res3","tenant_id":"ten1","name":"Never","kind":"user"}`)
	seed(t, db, "protections", "p1", `{"id":"p1","resource_id":"res1","tenant_id":"ten1"}`)
	seed(t, db, "protections", "p2", `{"id":"p2","resource_id":"res2","tenant_id":"ten1"}`)
	seed(t, db, "protections", "p3", `{"id":"p3","resource_id":"res3","tenant_id":"ten1"}`)
	seed(t, db, "archives", "a1", fmt.Sprintf(`{"id":"a1","tenant_id":"ten1","resource_id":"res1","created_at":"%s"}`, fresh))
	seed(t, db, "archives", "a2old", fmt.Sprintf(`{"id":"a2old","tenant_id":"ten1","resource_id":"res2","created_at":"%s"}`, old))
	return path
}

func TestBackupStaleDetectsStaleAndNever(t *testing.T) {
	path := seedStaleFixture(t)
	out, err := runNovel(t, newNovelBackupStaleCmd, "--db", path, "--max-age", "48h")
	if err != nil {
		t.Fatalf("backup-stale: %v", err)
	}
	view := decodeView[backupStaleView](t, out)
	if view.ProtectedScanned != 3 {
		t.Errorf("protected_scanned = %d, want 3", view.ProtectedScanned)
	}
	got := map[string]backupStaleItem{}
	for _, it := range view.Items {
		got[it.ResourceID] = it
	}
	if _, ok := got["res1"]; ok {
		t.Errorf("res1 has a fresh archive and must not be flagged: %s", out)
	}
	if it, ok := got["res2"]; !ok || it.NeverArchived || it.AgeHours < 90 {
		t.Errorf("res2 should be stale with age ~100h: %+v", got["res2"])
	}
	if it, ok := got["res3"]; !ok || !it.NeverArchived {
		t.Errorf("res3 should be never_archived: %+v", got["res3"])
	}
}

func TestBackupStaleDayWeekSuffix(t *testing.T) {
	path := seedStaleFixture(t)
	// 7d exceeds the 100h-old archive — only res3 (never archived) flagged.
	out, err := runNovel(t, newNovelBackupStaleCmd, "--db", path, "--max-age", "7d")
	if err != nil {
		t.Fatalf("backup-stale with 7d: %v", err)
	}
	view := decodeView[backupStaleView](t, out)
	if len(view.Items) != 1 || !view.Items[0].NeverArchived {
		t.Errorf("with --max-age 7d only res3 should be flagged: %s", out)
	}
}

func TestBackupStaleBadDuration(t *testing.T) {
	_, path := newTestStore(t)
	_, err := runNovel(t, newNovelBackupStaleCmd, "--db", path, "--max-age", "banana")
	if err == nil {
		t.Fatal("expected usage error for bad --max-age")
	}
}

func TestBackupStaleTenantFilter(t *testing.T) {
	path := seedStaleFixture(t)
	out, err := runNovel(t, newNovelBackupStaleCmd, "--db", path, "--max-age", "48h", "--tenant", "other")
	if err != nil {
		t.Fatalf("backup-stale: %v", err)
	}
	view := decodeView[backupStaleView](t, out)
	if len(view.Items) != 0 {
		t.Errorf("tenant filter should drop all rows: %s", out)
	}
}

func TestBackupStaleEmptyStoreNote(t *testing.T) {
	_, path := newTestStore(t)
	out, err := runNovel(t, newNovelBackupStaleCmd, "--db", path, "--max-age", "48h")
	if err != nil {
		t.Fatalf("backup-stale: %v", err)
	}
	view := decodeView[backupStaleView](t, out)
	if view.ProtectedScanned != 0 || view.Note == "" {
		t.Errorf("empty store should note fleet-sync: %s", out)
	}
}

func TestBackupStaleLimitCapsAllPaths(t *testing.T) {
	db, path := newTestStore(t)
	// 10 protected resources, none ever archived — every row takes the
	// never_archived append path, which previously bypassed --limit.
	for i := 0; i < 10; i++ {
		id := fmt.Sprintf("res%02d", i)
		seed(t, db, "resources", id, fmt.Sprintf(`{"id":"%s","tenant_id":"ten1","name":"R%d","kind":"user"}`, id, i))
		seed(t, db, "protections", "p"+id, fmt.Sprintf(`{"id":"p%s","resource_id":"%s","tenant_id":"ten1"}`, id, id))
	}
	out, err := runNovel(t, newNovelBackupStaleCmd, "--db", path, "--max-age", "48h", "--limit", "3")
	if err != nil {
		t.Fatalf("backup-stale: %v", err)
	}
	view := decodeView[backupStaleView](t, out)
	if len(view.Items) != 3 {
		t.Errorf("--limit 3 must cap never_archived rows too: got %d", len(view.Items))
	}
	if view.Note == "" {
		t.Errorf("capped run should set the capped note")
	}
}
