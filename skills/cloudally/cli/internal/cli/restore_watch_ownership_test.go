// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cloudally-pp-cli/internal/store"
)

// watchRow returns the single row a one-mailbox watch reports.
func watchRow(t *testing.T, w watchOutput) restoreJob {
	t.Helper()
	if len(w.Rows) != 1 {
		t.Fatalf("watch rows = %+v, want one", w.Rows)
	}
	return w.Rows[0]
}

// A deliberate --new-batch re-restore of the same mailbox and backup within
// the two-minute slack must not read the earlier batch's RESTORE task: batch
// #1 owns rt-1 (COMPLETED), batch #2 owns rt-2 (FAILED). #2 reports its own
// failure (exit 5), #1 its own success (exit 0).
func TestRestoreWatchNeverReadsAnotherBatchsTask(t *testing.T) {
	isolateAuthState(t)
	api := newFakeRestoreAPI(t, 0)
	first := submitOneMailbox(t, "a@contoso.com")
	// Watching #1 while rt-1 runs pins rt-1 to #1's row.
	if w, _ := watchJSON(t, first, "--once"); watchRow(t, w).TaskID != "rt-1" {
		t.Fatalf("batch #1 should pin rt-1, got %+v", w.Rows)
	}

	stdout, stderr, err := runAuthCmd(t, "", submitArgs("--date", "2026-10-01", "--entities", "a@contoso.com", "--new-batch")...)
	if err != nil {
		t.Fatalf("--new-batch submit: %v (%s)", err, stderr)
	}
	second := decodeSubmitOutput(t, stdout).BatchID
	api.set(func(f *fakeRestoreAPI) { f.statusByID = map[string]string{"rt-1": "COMPLETED", "rt-2": "FAILED"} })

	w, err := watchJSON(t, second, "--once")
	if row := watchRow(t, w); row.TaskID != "rt-2" || row.Status != "FAILED" {
		t.Fatalf("batch #2 read task %s (%s), want its own rt-2 FAILED", row.TaskID, row.Status)
	}
	if code := ExitCode(err); code != 5 {
		t.Fatalf("batch #2 exit = %d, want 5 (its restore failed): %v", code, err)
	}

	w, err = watchJSON(t, first, "--once")
	if row := watchRow(t, w); row.TaskID != "rt-1" || row.Status != "COMPLETED" {
		t.Fatalf("batch #1 read task %s (%s), want its own rt-1 COMPLETED", row.TaskID, row.Status)
	}
	if err != nil {
		t.Fatalf("batch #1 exit = %d, want 0: %v", ExitCode(err), err)
	}
	// Watching again changes nothing: pinned rows keep their tasks.
	if w, _ := watchJSON(t, second, "--once"); watchRow(t, w).TaskID != "rt-2" {
		t.Fatalf("batch #2 re-watch moved off rt-2: %+v", w.Rows)
	}
}

// The matcher's rules, directly: tasks pinned elsewhere are skipped, a task
// created at or after the row's submission is preferred, two eligible tasks
// are never guessed between, and a pinned row only ever takes its own task.
func TestMatchRestoreTasksOwnership(t *testing.T) {
	base := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	row := submittedJob(1, "a@contoso.com", 20261001020000, base)
	task := func(id string, created time.Time) restoreTaskView {
		return restoreTaskView{ID: flexString(id), Account: "a@contoso.com", Source: "MS365", Status: "COMPLETED",
			SnapshotDate: 20261001020000, CreateDate: created.Format(time.RFC3339)}
	}
	earlier := task("rt-earlier", base.Add(-30*time.Second)) // inside the slack, before this submit
	own := task("rt-own", base.Add(2*time.Second))
	later := task("rt-later", base.Add(5*time.Second))

	cases := []struct {
		name   string
		tasks  []restoreTaskView
		pinned map[string]bool
		want   string
	}{
		{"prefers the task created at or after the submit", []restoreTaskView{earlier, own}, nil, "rt-own"},
		{"a task inside the slack counts when it is the only one", []restoreTaskView{earlier}, nil, "rt-earlier"},
		{"skips a task another batch pinned", []restoreTaskView{earlier, own}, map[string]bool{"rt-own": true}, "rt-earlier"},
		{"two tasks after the submit: no guess", []restoreTaskView{own, later}, nil, ""},
		{"two tasks inside the slack only: no guess", []restoreTaskView{earlier, task("rt-earlier-2", base.Add(-20*time.Second))}, nil, ""},
		{"the only candidate pinned elsewhere: no match", []restoreTaskView{own}, map[string]bool{"rt-own": true}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := matchRestoreTasksExcluding([]restoreJob{row}, tc.tasks, "MS365", tc.pinned)[1]
			gotID := ""
			if got != nil {
				gotID = string(got.ID)
			}
			if gotID != tc.want {
				t.Fatalf("matched %q, want %q", gotID, tc.want)
			}
		})
	}

	pinnedRow := row
	pinnedRow.TaskID = "rt-earlier"
	if got := matchRestoreTasksExcluding([]restoreJob{pinnedRow}, []restoreTaskView{own, earlier}, "MS365", map[string]bool{"rt-earlier": true})[1]; got == nil || string(got.ID) != "rt-earlier" {
		t.Fatalf("a pinned row must keep its own task, got %v", got)
	}
	if got := matchRestoreTasksExcluding([]restoreJob{pinnedRow}, []restoreTaskView{own}, "MS365", nil)[1]; got != nil {
		t.Fatalf("a pinned row whose task is not listed was re-matched to %s", got.ID)
	}
}

// seedLedgerRow writes one batch and one row with a recorded RESTORE task
// into the store at path.
func seedLedgerRow(t *testing.T, path, batchID, taskID string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	st, err := store.OpenWithContext(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := ledgerInsertBatch(st.DB(), restoreBatch{BatchID: batchID, BatchKey: "k-" + batchID, CreatedAt: time.Now(), Scope: scopeAccount, TaskID: "64b0e9", Dates: []string{"2026-10-01"}, Options: map[string]any{}}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.DB().Exec(`INSERT INTO restore_jobs (batch_id, entity, requested, resolved_raw, resolved, status, task_id, updated_at)
		VALUES (?, 'a@contoso.com', '2026-10-01', 20261001020000, '2026-10-01T02:00:00Z', 'COMPLETED', ?, ?)`, batchID, taskID, isoUTC(time.Now())); err != nil {
		t.Fatal(err)
	}
}

// Tasks pinned by another batch count wherever that batch lives: in this
// login's ledger, or in another login's ledger in the same data directory.
// This batch's own pins do not count, and an unreadable ledger fails the
// watch closed rather than risking a task another batch owns.
func TestRestoreTasksPinnedElsewhereReadsEveryLedger(t *testing.T) {
	isolateAuthState(t)
	resetStoreScope(t)
	t.Setenv("CLOUDALLY_ACCESS_TOKEN", "tenant-token")
	t.Setenv("CLOUDALLY_CLIENT_ID", "cid")
	t.Setenv("CLOUDALLY_CLIENT_SECRET", "csecret")
	configureDefaultDBScope("")
	own := defaultDBPath(restoreCLIName)
	seedLedgerRow(t, own, "this-batch", "rt-mine")
	seedLedgerRow(t, own, "other-batch", "rt-other-batch")
	otherLogin := filepath.Join(filepath.Dir(own), "data-otherlogin.db")
	seedLedgerRow(t, otherLogin, "other-login-batch", "rt-other-login")

	st, err := store.OpenWithContext(context.Background(), own)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	got, err := restoreTasksPinnedElsewhere(context.Background(), st.DB(), "this-batch")
	if err != nil {
		t.Fatal(err)
	}
	if !got["rt-other-batch"] || !got["rt-other-login"] || got["rt-mine"] {
		t.Fatalf("pinned elsewhere = %v, want rt-other-batch and rt-other-login, not rt-mine", got)
	}

	if err := os.WriteFile(otherLogin, []byte("not a database"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, sfx := range []string{"-wal", "-shm"} {
		_ = os.Remove(otherLogin + sfx)
	}
	if _, err := restoreTasksPinnedElsewhere(context.Background(), st.DB(), "this-batch"); err == nil || !strings.Contains(err.Error(), "cannot be read") {
		t.Fatalf("an unreadable ledger should fail the watch closed, got %v", err)
	}
}
