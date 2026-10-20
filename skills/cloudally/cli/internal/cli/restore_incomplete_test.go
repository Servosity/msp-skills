// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

// runAuthCmdCtx is runAuthCmd under a context the test controls, the way a
// SIGINT cancels a real run.
func runAuthCmdCtx(t *testing.T, ctx context.Context, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	var flags rootFlags
	root := newRootCmd(&flags)
	var out, errOut bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errOut)
	root.SetArgs(args)
	err = root.ExecuteContext(ctx)
	return out.String(), errOut.String(), err
}

// printedCommand returns the command a message prints after marker, up to
// and including its trailing --yes.
func printedCommand(t *testing.T, msg, marker string) []string {
	t.Helper()
	i := strings.Index(msg, marker)
	if i < 0 {
		t.Fatalf("message prints no command after %q:\n%s", marker, msg)
	}
	rest := msg[i+len(marker):]
	end := strings.Index(rest, " --yes")
	if end < 0 {
		t.Fatalf("printed command has no --yes:\n%s", rest)
	}
	fields := strings.Fields(rest[:end+len(" --yes")])
	if len(fields) < 3 || fields[0] != "cloudally-cli" {
		t.Fatalf("printed command = %q", rest[:end])
	}
	return fields[1:]
}

// Item 1(a): --timeout bounds each request, not the run, so a slow --rate
// still sends every row.
func TestRestoreSubmitTimeoutIsPerRequest(t *testing.T) {
	isolateAuthState(t)
	api := newFakeRestoreAPI(t, 0)
	args := []string{"restore", "submit", "--task", "64b0e9", "--date", "2026-10-01", "--entities", "a@contoso.com,b@contoso.com",
		"--concurrency", "1", "--rate", "0.5", "--timeout", "1s", "--yes", "--json"}
	if _, stderr, err := runAuthCmd(t, "", args...); err != nil {
		t.Fatalf("submit: %v (%s)", err, stderr)
	}
	if api.entityPosts("a@contoso.com") != 1 || api.entityPosts("b@contoso.com") != 1 {
		t.Fatalf("a run longer than --timeout left rows unsent: %v", api.byEntity)
	}
}

// Item 1(b, c): a run that stops with rows still PLANNED exits non-zero and
// prints the --resume command; watch calls the batch incomplete; the
// printed command sends exactly the unsent row.
func TestRestoreInterruptedSubmitIsIncomplete(t *testing.T) {
	isolateAuthState(t)
	api := newFakeRestoreAPI(t, 0)
	api.set(func(f *fakeRestoreAPI) { f.taskStatus = "COMPLETED" })
	args := []string{"restore", "submit", "--task", "64b0e9", "--date", "2026-10-01", "--entities", "a@contoso.com,b@contoso.com",
		"--concurrency", "1", "--rate", "0.5", "--yes", "--json"}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	stdout, _, err := runAuthCmdCtx(t, ctx, args...)
	if err == nil || ExitCode(err) != 5 || !strings.Contains(err.Error(), "1 never sent (still PLANNED)") || !strings.Contains(err.Error(), "1 accepted (those are restoring and are never sent again)") {
		t.Fatalf("interrupted submit: want exit 5 naming the unsent row, got %v", err)
	}
	if api.posts.Load() != 1 {
		t.Fatalf("posts = %d, want 1 before the interruption", api.posts.Load())
	}
	resume := printedCommand(t, err.Error(), "refused before they ran with: ")
	if !strings.Contains(strings.Join(resume, " "), "--resume") {
		t.Fatalf("printed command is not a resume: %v", resume)
	}
	batchID := decodeSubmitOutput(t, stdout).BatchID

	out, _, err := runAuthCmd(t, "", "restore", "watch", "--batch", batchID, "--once", "--json")
	if err == nil || ExitCode(err) != 5 || !strings.Contains(err.Error(), "is incomplete: 1 restore(s) were never submitted") {
		t.Fatalf("watch of an incomplete batch: want exit 5, got %v", err)
	}
	var w watchOutput
	if jerr := json.Unmarshal([]byte(out), &w); jerr != nil || w.Done || w.Tally.Unsent != 1 {
		t.Fatalf("watch output: done %v unsent %d (%v)\n%s", w.Done, w.Tally.Unsent, jerr, out)
	}

	if _, stderr, err := runAuthCmd(t, "", resume...); err != nil {
		t.Fatalf("printed resume %v: %v (%s)", resume, err, stderr)
	}
	if api.entityPosts("a@contoso.com") != 1 || api.entityPosts("b@contoso.com") != 1 {
		t.Fatalf("after the printed resume: %v", api.byEntity)
	}
	if _, _, err := runAuthCmd(t, "", "restore", "watch", "--batch", batchID, "--once", "--json"); err != nil {
		t.Fatalf("watch of the completed batch: %v", err)
	}
}

// Item 4: an UNKNOWN row is never settled by a task another batch created
// for another backup of the same mailbox.
func TestRestoreWatchIgnoresOtherBackupsTask(t *testing.T) {
	isolateAuthState(t)
	api := newFakeRestoreAPI(t, 0)
	api.set(func(f *fakeRestoreAPI) { f.taskStatus = "COMPLETED"; f.backupRaw = 20260831120000 })
	api.setStatus("a@contoso.com", http.StatusServiceUnavailable)
	stdout, _, err := runAuthCmd(t, "", "restore", "submit", "--task", "64b0e9", "--date", "2026-09-01", "--entities", "a@contoso.com", "--rate", "1000", "--yes", "--json")
	if err == nil {
		t.Fatal("B1 should report its UNKNOWN row")
	}
	b1 := decodeSubmitOutput(t, stdout).BatchID

	// B2: the same mailbox at another date is accepted and gets a task.
	api.setStatus("a@contoso.com", http.StatusOK)
	api.set(func(f *fakeRestoreAPI) { f.backupRaw = 20260930120000 })
	if _, stderr, err := runAuthCmd(t, "", "restore", "submit", "--task", "64b0e9", "--date", "2026-10-01", "--entities", "a@contoso.com", "--rate", "1000", "--yes", "--json"); err != nil {
		t.Fatalf("B2: %v (%s)", err, stderr)
	}

	_, _, err = runAuthCmd(t, "", "restore", "watch", "--batch", b1, "--once", "--json")
	if err == nil || ExitCode(err) != 5 || !strings.Contains(err.Error(), "still UNKNOWN") {
		t.Fatalf("watching B1: want exit 5 with its row still UNKNOWN, got %v", err)
	}
	var row restoreJob
	for _, j := range ledgerRows(t) {
		if j.BatchID == b1 {
			row = j
		}
	}
	if row.SubmitStatus != jobUnknown || row.TaskID != "" {
		t.Fatalf("B1's row took B2's task: submit %s task %q status %s", row.SubmitStatus, row.TaskID, row.Status)
	}

	// --retry-unknown can still send the 09-01 restore that never happened.
	api.set(func(f *fakeRestoreAPI) { f.backupRaw = 20260831120000 })
	if _, stderr, err := runAuthCmd(t, "", "restore", "submit", "--task", "64b0e9", "--date", "2026-09-01", "--entities", "a@contoso.com", "--rate", "1000", "--yes", "--json", "--resume", "--retry-unknown"); err != nil {
		t.Fatalf("retry-unknown: %v (%s)", err, stderr)
	}
	if got := api.entityPosts("a@contoso.com"); got != 3 {
		t.Fatalf("a posts = %d, want 3 (B1 503, B2, B1 retry)", got)
	}
}

// Item 4, the other direction: a task whose snapshotDate is within the
// window of the row's resolved backup still settles an UNKNOWN row.
func TestMatchRestoreTasksSnapshotWindow(t *testing.T) {
	base := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	unknown := restoreJob{ID: 1, Entity: "a@contoso.com", ResolvedRaw: 20261001020000, Status: jobUnknown, SubmitStatus: jobUnknown, SubmittedAt: &base}
	near := restoreTaskView{ID: "near", Account: "a@contoso.com", Source: "MS365", Status: "COMPLETED", SnapshotDate: 20261001015900, CreateDate: base.Add(time.Second).Format(time.RFC3339)}
	other := restoreTaskView{ID: "other", Account: "a@contoso.com", Source: "MS365", Status: "COMPLETED", SnapshotDate: 20260930120000, CreateDate: base.Add(time.Second).Format(time.RFC3339)}
	noSnap := restoreTaskView{ID: "nosnap", Account: "a@contoso.com", Source: "MS365", Status: "COMPLETED", CreateDate: base.Add(time.Second).Format(time.RFC3339)}
	if m := matchRestoreTasks([]restoreJob{unknown}, []restoreTaskView{other, near}, "MS365")[1]; m == nil || string(m.ID) != "near" {
		t.Fatalf("UNKNOWN row should settle on the task for its own backup, got %v", m)
	}
	if m := matchRestoreTasks([]restoreJob{unknown}, []restoreTaskView{other, noSnap}, "MS365")[1]; m != nil {
		t.Fatalf("UNKNOWN row settled on %s without a snapshot match", m.ID)
	}
	submitted := unknown
	submitted.Status, submitted.SubmitStatus = jobSubmitted, jobSubmitted
	if m := matchRestoreTasks([]restoreJob{submitted}, []restoreTaskView{other, noSnap}, "MS365")[1]; m == nil || string(m.ID) != "nosnap" {
		t.Fatalf("a confirmed row may take a task with no snapshotDate but never another backup's, got %v", m)
	}
}
