// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// cli-printing-press: novel-scaffold-test
// Novel command scaffold tests. Keep the wiring smoke test and add behavior cases as needed.

package cli

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"cloudally-pp-cli/internal/cliutil/testenv"
)

// TestNovelRestoreWatchHelpWires smoke-tests that the restore watch command
// resolves at runtime and renders useful --help output. Catches wiring
// regressions (missing AddCommand, panicking RunE on --help, etc.) before
// review. Keep this smoke test when adding behavior-specific cases.
func TestNovelRestoreWatchHelpWires(t *testing.T) {
	testenv.Isolate(t)
	cmd := RootCmd()
	cmd.SetArgs([]string{"restore", "watch", "--help"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("restore watch --help error = %v (novel command not wired correctly?)", err)
	}
	help := out.String()
	for _, want := range []string{"Usage:", "watch"} {
		if !strings.Contains(help, want) {
			t.Fatalf("restore watch --help missing %q in output:\n%s", want, help)
		}
	}
}

func submittedJob(id int64, entity string, raw int64, at time.Time) restoreJob {
	return restoreJob{ID: id, Entity: entity, ResolvedRaw: raw, Status: jobSubmitted, SubmittedAt: &at}
}

func TestMatchRestoreTasks(t *testing.T) {
	base := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	jobs := []restoreJob{
		submittedJob(1, "jane.doe@contoso.com", 20261001120000, base),
		submittedJob(2, "jane.doe@contoso.com", 20260930180000, base.Add(time.Second)),
		submittedJob(3, "john.smith@contoso.com", 20260928230000, base.Add(2*time.Second)),
		{ID: 4, Entity: "failed@contoso.com", Status: jobFailed},
		submittedJob(5, "nobody@contoso.com", 20261001000000, base),
	}
	tasks := []restoreTaskView{
		// Old restore for Jane from yesterday: outside the slack window.
		{ID: "old", Account: "jane.doe@contoso.com", Source: "MS365", Status: "ARCHIVED", CreateDate: base.Add(-24 * time.Hour).Format(time.RFC3339)},
		// Snapshot match for job 2 even though it was created after job 1's candidate.
		{ID: "t-jane-sep30", Account: "JANE.DOE@contoso.com", Source: "MS365", Status: "INPROCESS", SnapshotDate: 20260930180000, CreateDate: base.Add(30 * time.Second).Format(time.RFC3339), Progress: &taskProgress{Current: 40, State: "RUNNING"}},
		{ID: "t-jane-oct1", Account: "jane.doe@contoso.com", Source: "MS365", Status: "QUEUED", CreateDate: base.Add(10 * time.Second).Format(time.RFC3339)},
		// Wrong source: never matched.
		{ID: "t-john-gmail", Account: "john.smith@contoso.com", Source: "GMAIL", Status: "QUEUED", CreateDate: base.Add(5 * time.Second).Format(time.RFC3339)},
		// Within the two minute slack even though created before submit.
		{ID: "t-john", Account: "john.smith@contoso.com", Source: "MS365", Status: "ARCHIVED", CreateDate: base.Add(-90 * time.Second).Format(time.RFC3339)},
		// Unparseable createDate: skipped.
		{ID: "t-bad", Account: "nobody@contoso.com", Source: "MS365", Status: "QUEUED", CreateDate: "soon"},
		// Failed rows are never matched even when a task exists.
		{ID: "t-failed", Account: "failed@contoso.com", Source: "MS365", Status: "QUEUED", CreateDate: base.Format(time.RFC3339)},
	}
	got := matchRestoreTasks(jobs, tasks, "MS365")
	want := map[int64]string{1: "t-jane-oct1", 2: "t-jane-sep30", 3: "t-john"}
	for id, taskID := range want {
		m, ok := got[id]
		if !ok || string(m.ID) != taskID {
			t.Fatalf("job %d matched %v, want %s", id, m, taskID)
		}
	}
	for _, id := range []int64{4, 5} {
		if m, ok := got[id]; ok {
			t.Fatalf("job %d should be unmatched, got %s", id, m.ID)
		}
	}
	if got[2].Progress.String() != "RUNNING 40%" {
		t.Fatalf("progress = %q", got[2].Progress.String())
	}
}

func TestMatchRestoreTasksPrefersRecordedTaskID(t *testing.T) {
	base := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	j := submittedJob(1, "jane.doe@contoso.com", 20261001120000, base)
	j.TaskID = "second"
	j.Status = "QUEUED"
	tasks := []restoreTaskView{
		{ID: "first", Account: "jane.doe@contoso.com", Source: "MS365", Status: "ARCHIVED", CreateDate: base.Add(time.Second).Format(time.RFC3339)},
		{ID: "second", Account: "jane.doe@contoso.com", Source: "MS365", Status: "INPROCESS", CreateDate: base.Add(2 * time.Second).Format(time.RFC3339)},
	}
	got := matchRestoreTasks([]restoreJob{j}, tasks, "MS365")
	if m := got[1]; m == nil || string(m.ID) != "second" {
		t.Fatalf("recorded task id should win, got %v", m)
	}
}

func TestRestoreRunningStatusTable(t *testing.T) {
	for _, s := range []string{"PENDING", "queued", "Dispatched", "INPROCESS", "ACTIVE"} {
		if !restoreTaskRunning(s) {
			t.Fatalf("%s should be running", s)
		}
	}
	for _, s := range []string{"ARCHIVED", "NOTACTIVE", "IN_ANOTHER", "PAUSED", "ON_PREMISE", "SKIPPED", "", "weird"} {
		if restoreTaskRunning(s) {
			t.Fatalf("%s should be finished", s)
		}
	}
	jobs := []restoreJob{
		{Status: jobFailed}, {Status: jobFailedRedirect}, {Status: jobUnmatched}, {Status: jobSubmitted}, {Status: jobPlanned},
		{Status: "INPROCESS"}, {Status: "ARCHIVED"},
	}
	tally := tallyJobs(jobs)
	if tally.Failed != 2 || tally.Unmatched != 1 || tally.Pending != 1 || tally.Unsent != 1 || tally.Running != 1 || tally.Finished != 1 {
		t.Fatalf("tally = %+v", tally)
	}
	if !jobTerminal(restoreJob{Status: "ARCHIVED"}) || jobTerminal(restoreJob{Status: "QUEUED"}) || jobTerminal(restoreJob{Status: jobSubmitted}) {
		t.Fatal("jobTerminal mismatch")
	}
}

func TestRestoreWatchDryRunSkipsLedger(t *testing.T) {
	testenv.Isolate(t)
	cmd := RootCmd()
	cmd.SetArgs([]string{"restore", "watch", "--batch", "20261015-120000-ab12", "--once", "--dry-run", "--json"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("dry-run error = %v", err)
	}
	if !strings.Contains(out.String(), `"dry_run":true`) || !strings.Contains(out.String(), "20261015-120000-ab12") {
		t.Fatalf("unexpected output: %s", out.String())
	}
}

func TestRestoreWatchUnknownBatchIsUsageError(t *testing.T) {
	testenv.Isolate(t)
	cmd := RootCmd()
	cmd.SetArgs([]string{"restore", "watch", "--batch", "20261015-000000-ffff", "--once"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "unknown batch") {
		t.Fatalf("want unknown batch usage error, got %v", err)
	}
	if code := ExitCode(err); code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
}

func TestWatchTallyKeepsUnknownSeparate(t *testing.T) {
	jobs := []restoreJob{{Status: jobUnknown}, {Status: jobSubmitting}, {Status: jobSubmitted}, {Status: "COMPLETED"}}
	tally := tallyJobs(jobs)
	if tally.Unknown != 2 || tally.Pending != 1 || tally.Finished != 1 {
		t.Fatalf("tally = %+v", tally)
	}
	if !jobOutcomeUnknown(jobs[0]) || !jobOutcomeUnknown(jobs[1]) || jobOutcomeUnknown(jobs[2]) {
		t.Fatal("UNKNOWN and SUBMITTING rows should count as unconfirmed")
	}
}
