// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
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

func TestBackupStaleFlagsNeverArchived(t *testing.T) {
	path := seedStaleFixture(t)
	out, err := runNovel(t, newNovelBackupStaleCmd, "--db", path)
	if err != nil {
		t.Fatalf("backup-stale: %v", err)
	}
	view := decodeView[backupStaleView](t, out)
	if view.ProtectedScanned != 3 {
		t.Errorf("protected_scanned = %d, want 3", view.ProtectedScanned)
	}
	// res1 and res2 have archives (their age is not judged); res3 has none.
	if len(view.Items) != 1 || view.Items[0].ResourceID != "res3" || !view.Items[0].NeverArchived || view.Items[0].Reason != "never_archived" {
		t.Errorf("only res3 should be flagged, as never_archived: %s", out)
	}
}

// --max-age is kept for old scripts and ignored.
func TestBackupStaleMaxAgeIsIgnored(t *testing.T) {
	path := seedStaleFixture(t)
	plain, err := runNovel(t, newNovelBackupStaleCmd, "--db", path)
	if err != nil {
		t.Fatalf("backup-stale: %v", err)
	}
	for _, v := range []string{"48h", "7d", "banana"} {
		out, err := runNovel(t, newNovelBackupStaleCmd, "--db", path, "--max-age", v)
		if err != nil {
			t.Fatalf("backup-stale --max-age %s: %v", v, err)
		}
		if len(decodeView[backupStaleView](t, out).Items) != len(decodeView[backupStaleView](t, plain).Items) {
			t.Errorf("--max-age %s changed the result: %s", v, out)
		}
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

// Codex r1 (afi review) class archive-creation-read-as-backup-time: spec.json
// v1Archive is a backup sequence (a full backup and the incremental backups
// after it) and its created_at dates the archive, not its latest backup, so a
// long-lived archive is not a stale backup. What the store can show is a
// protected resource with no archive at all, and a tenant where no backup task
// finished in the synced task-statistics window.
func TestBackupStaleDoesNotReadArchiveAgeAsBackupAge(t *testing.T) {
	db, path := newTestStore(t)
	old := time.Now().UTC().AddDate(0, 0, -30).Format(time.RFC3339)
	win := `"window_start":"2026-10-21T00:00:00Z","window_end":"2026-10-28T00:00:00Z"`
	seed(t, db, "resources", "res1", `{"id":"res1","tenant_id":"ten1","name":"Long-lived","kind":"office_user"}`)
	seed(t, db, "protections", "p1", `{"id":"p1","resource_id":"res1","tenant_id":"ten1"}`)
	seed(t, db, "archives", "a1", fmt.Sprintf(`{"id":"a1","tenant_id":"ten1","resource_id":"res1","created_at":"%s"}`, old))
	seed(t, db, "task_stats", "ten1", `{"tenant_id":"ten1","total":{"done":14,"failed":0,"warnings":1},`+win+`}`)
	seed(t, db, "resources", "res2", `{"id":"res2","tenant_id":"ten2","name":"Silent","kind":"office_user"}`)
	seed(t, db, "protections", "p2", `{"id":"p2","resource_id":"res2","tenant_id":"ten2"}`)
	seed(t, db, "archives", "a2", fmt.Sprintf(`{"id":"a2","tenant_id":"ten2","resource_id":"res2","created_at":"%s"}`, old))
	seed(t, db, "task_stats", "ten2", `{"tenant_id":"ten2","total":{"done":0,"failed":3,"warnings":0},`+win+`}`)
	out, err := runNovel(t, newNovelBackupStaleCmd, "--db", path)
	if err != nil {
		t.Fatalf("backup-stale: %v", err)
	}
	got := map[string]backupStaleItem{}
	for _, it := range decodeView[backupStaleView](t, out).Items {
		got[it.ResourceID] = it
	}
	if _, ok := got["res1"]; ok {
		t.Errorf("a resource whose tenant finished backups this week was flagged for its archive's age: %s", out)
	}
	if it, ok := got["res2"]; !ok || it.Reason != "no_finished_task_in_window" || it.NeverArchived {
		t.Errorf("a protected resource in a tenant with no finished task is not flagged: %s", out)
	}
}
