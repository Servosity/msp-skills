// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"cloudally-pp-cli/internal/store"
)

func decodeSubmitOutput(t *testing.T, stdout string) submitOutput {
	t.Helper()
	var out submitOutput
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("decode submit output: %v\n%s", err, stdout)
	}
	return out
}

func ledgerRowByEntity(t *testing.T, entity string) restoreJob {
	t.Helper()
	for _, j := range ledgerRows(t) {
		if j.Entity == entity {
			return j
		}
	}
	t.Fatalf("no ledger row for %s", entity)
	return restoreJob{}
}

// TestRestoreWatchRemoteFailedIsNeverResent is the reviewer's repro: a
// restore the API accepted whose RESTORE task then reported FAILED must
// not be sent again by --resume, while a restore the API rejected before
// it ran still is.
func TestRestoreWatchRemoteFailedIsNeverResent(t *testing.T) {
	isolateAuthState(t)
	api := newFakeRestoreAPI(t, 0)
	api.setStatus("b@contoso.com", http.StatusBadRequest)
	api.set(func(f *fakeRestoreAPI) { f.taskStatus = "FAILED" })
	args := []string{"restore", "submit", "--task", "64b0e9", "--date", "2026-10-01", "--entities", "a@contoso.com,b@contoso.com",
		"--concurrency", "1", "--rate", "1000", "--yes", "--json"}
	stdout, _, err := runAuthCmd(t, "", args...)
	if err == nil || ExitCode(err) != 5 {
		t.Fatalf("first submit: want exit 5 for the rejected row, got %v", err)
	}
	batchID := decodeSubmitOutput(t, stdout).BatchID

	_, _, err = runAuthCmd(t, "", "restore", "watch", "--batch", batchID, "--once", "--json")
	if err == nil || ExitCode(err) != 5 || !strings.Contains(err.Error(), "failed after CloudAlly accepted them") {
		t.Fatalf("watch: want exit 5 naming the accepted failure, got %v", err)
	}
	a := ledgerRowByEntity(t, "a@contoso.com")
	if a.Status != jobFailed || a.SubmitStatus != jobSubmitted || a.TaskID == "" {
		t.Fatalf("a after watch: status %q submit %q task %q; want FAILED shown, SUBMITTED kept, task recorded", a.Status, a.SubmitStatus, a.TaskID)
	}
	if b := ledgerRowByEntity(t, "b@contoso.com"); b.Status != jobFailed || b.SubmitStatus != jobFailed {
		t.Fatalf("b after watch: status %q submit %q; want FAILED/FAILED", b.Status, b.SubmitStatus)
	}

	api.setStatus("b@contoso.com", http.StatusOK)
	if _, stderr, err := runAuthCmd(t, "", append(args, "--resume")...); err != nil {
		t.Fatalf("resume: %v (%s)", err, stderr)
	}
	if got := api.entityPosts("a@contoso.com"); got != 1 {
		t.Fatalf("a was accepted and its task failed; resume must not send it again, posts = %d", got)
	}
	if got := api.entityPosts("b@contoso.com"); got != 2 {
		t.Fatalf("b was rejected before it ran; resume must send it again, posts = %d", got)
	}

	// History and watch still show the task's own status.
	hist, _, err := runAuthCmd(t, "", "restore", "history", "--batch", batchID, "--json")
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	var rows []restoreJob
	if err := json.Unmarshal([]byte(hist), &rows); err != nil {
		t.Fatalf("decode history: %v\n%s", err, hist)
	}
	for _, r := range rows {
		if r.Entity == "a@contoso.com" && (r.Status != jobFailed || r.SubmitStatus != jobSubmitted) {
			t.Fatalf("history row a = status %q submit %q", r.Status, r.SubmitStatus)
		}
	}
}

// TestRestoreWatchSettledUnknownIsNeverResent: an UNKNOWN row watch matched
// to a RESTORE task was accepted, so even --retry-unknown leaves it alone
// after that task reports FAILED.
func TestRestoreWatchSettledUnknownIsNeverResent(t *testing.T) {
	isolateAuthState(t)
	api := newFakeRestoreAPI(t, 0)
	api.setStatus("b@contoso.com", http.StatusBadGateway)
	api.set(func(f *fakeRestoreAPI) { f.acceptAnyway["b@contoso.com"] = true; f.taskStatus = "FAILED" })
	args := []string{"restore", "submit", "--task", "64b0e9", "--date", "2026-10-01", "--entities", "a@contoso.com,b@contoso.com",
		"--concurrency", "1", "--rate", "1000", "--yes", "--json"}
	stdout, _, err := runAuthCmd(t, "", args...)
	if err == nil || ExitCode(err) != 5 {
		t.Fatalf("first submit: want exit 5, got %v", err)
	}
	batchID := decodeSubmitOutput(t, stdout).BatchID
	if b := ledgerRowByEntity(t, "b@contoso.com"); b.SubmitStatus != jobUnknown {
		t.Fatalf("b after a 502: submit %q, want UNKNOWN", b.SubmitStatus)
	}
	_, _, _ = runAuthCmd(t, "", "restore", "watch", "--batch", batchID, "--once", "--json")
	if b := ledgerRowByEntity(t, "b@contoso.com"); b.SubmitStatus != jobSubmitted || b.Status != jobFailed || b.TaskID == "" {
		t.Fatalf("b after watch: status %q submit %q task %q; want FAILED shown, settled SUBMITTED", b.Status, b.SubmitStatus, b.TaskID)
	}
	api.setStatus("b@contoso.com", http.StatusOK)
	if _, stderr, err := runAuthCmd(t, "", append(args, "--resume", "--retry-unknown")...); err != nil {
		t.Fatalf("resume --retry-unknown: %v (%s)", err, stderr)
	}
	if got := api.entityPosts("b@contoso.com"); got != 1 {
		t.Fatalf("watch found b's task, so it was accepted; posts = %d, want 1", got)
	}
}

// TestRestoreLedgerBackfillsSubmitStatus models rows an older binary wrote,
// when watch still put the task status over the submission outcome: a
// FAILED row with a task id was accepted, a FAILED row without one and
// with a 4xx was a clean rejection.
func TestRestoreLedgerBackfillsSubmitStatus(t *testing.T) {
	isolateAuthState(t)
	api := newFakeRestoreAPI(t, 0)
	args := []string{"restore", "submit", "--task", "64b0e9", "--date", "2026-10-01", "--entities", "a@contoso.com,b@contoso.com,c@contoso.com",
		"--rate", "1000", "--yes", "--json"}
	stdout, stderr, err := runAuthCmd(t, "", args...)
	if err != nil {
		t.Fatalf("submit: %v (%s)", err, stderr)
	}
	batchID := decodeSubmitOutput(t, stdout).BatchID
	ledgerExec(t, `UPDATE restore_jobs SET submit_status = '', status = 'FAILED', task_id = 'rt-1', http_status = 200 WHERE entity = ?`, "a@contoso.com")
	ledgerExec(t, `UPDATE restore_jobs SET submit_status = '', status = 'FAILED', task_id = NULL, http_status = 400 WHERE entity = ?`, "b@contoso.com")
	guardReject(t, "b@contoso.com")
	ledgerExec(t, `UPDATE restore_jobs SET submit_status = '', status = 'ARCHIVED', task_id = 'rt-3' WHERE entity = ?`, "c@contoso.com")
	if _, stderr, err := runAuthCmd(t, "", append(args, "--resume")...); err != nil {
		t.Fatalf("resume: %v (%s)", err, stderr)
	}
	if api.entityPosts("a@contoso.com") != 1 || api.entityPosts("b@contoso.com") != 2 || api.entityPosts("c@contoso.com") != 1 {
		t.Fatalf("posts a=%d b=%d c=%d, want 1, 2, 1", api.entityPosts("a@contoso.com"), api.entityPosts("b@contoso.com"), api.entityPosts("c@contoso.com"))
	}
	if a := ledgerRowByEntity(t, "a@contoso.com"); a.SubmitStatus != jobSubmitted || a.Status != jobFailed {
		t.Fatalf("a backfilled to submit %q status %q", a.SubmitStatus, a.Status)
	}

	// A ledger that has never been opened read-write by this binary has no
	// submit_status column; read-only history derives it the same way.
	ledgerExec(t, `UPDATE restore_jobs SET status = 'FAILED', task_id = 'rt-1' WHERE entity = ?`, "a@contoso.com")
	ledgerExec(t, `ALTER TABLE restore_jobs DROP COLUMN submit_status`)
	hist, _, err := runAuthCmd(t, "", "restore", "history", "--batch", batchID, "--json")
	if err != nil {
		t.Fatalf("history on a ledger without submit_status: %v", err)
	}
	var rows []restoreJob
	if err := json.Unmarshal([]byte(hist), &rows); err != nil || len(rows) != 3 {
		t.Fatalf("history = %s (%v)", hist, err)
	}
	for _, r := range rows {
		if r.Entity == "a@contoso.com" && r.SubmitStatus != jobSubmitted {
			t.Fatalf("derived submit_status for a = %q, want SUBMITTED", r.SubmitStatus)
		}
	}
	ro, ok, err := openLedgerRead(context.Background())
	if err != nil || !ok {
		t.Fatalf("open read-only: ok=%v err=%v", ok, err)
	}
	if ledgerHasColumn(ro.DB(), "restore_jobs", "submit_status") {
		t.Fatal("history must not have migrated the ledger; the no-column path was not exercised")
	}
	ro.Close()
	list, _, err := runAuthCmd(t, "", "restore", "history", "--json")
	if err != nil {
		t.Fatalf("history list: %v", err)
	}
	var batches []batchSummary
	if err := json.Unmarshal([]byte(list), &batches); err != nil || len(batches) != 1 || batches[0].Submitted != 3 || batches[0].Failed != 1 {
		t.Fatalf("history list = %s (%v)", list, err)
	}
}

// TestLegacySubmitStatusRuleAgrees keeps the Go fallback and the SQL the
// migration runs on the same rule.
func TestLegacySubmitStatusRuleAgrees(t *testing.T) {
	isolateAuthState(t)
	db, err := openLedger(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	cases := []struct {
		status, taskID string
		http           int
	}{
		{jobPlanned, "", 0}, {jobSubmitting, "", 0}, {jobSubmitted, "", 200}, {jobUnknown, "", 502}, {jobFailedRedirect, "", 200},
		{jobFailed, "", 400}, {jobFailed, "", 0}, {jobFailed, "", 200}, {jobFailed, "rt-1", 502}, {jobFailed, "rt-1", 200},
		{jobUnmatched, "", 200}, {"QUEUED", "rt-2", 200}, {"ARCHIVED", "rt-3", 0}, {jobUnknown, "rt-4", 0},
	}
	for _, tc := range cases {
		var fromSQL string
		q := `SELECT ` + store.LegacySubmitStatusSQL("") + ` FROM (SELECT ? AS status, ? AS task_id, ? AS http_status)`
		if err := db.DB().QueryRow(q, tc.status, tc.taskID, tc.http).Scan(&fromSQL); err != nil {
			t.Fatal(err)
		}
		fromGo := restoreJob{Status: tc.status, TaskID: tc.taskID, HTTPStatus: tc.http}.submitOutcome()
		if fromSQL != fromGo {
			t.Fatalf("%+v: SQL says %s, Go says %s", tc, fromSQL, fromGo)
		}
	}
}

// TestRestoreSubmitResumeKeepsRecordedOptions: resumed rows go where the
// batch said they would, and a conflicting option is refused.
func TestRestoreSubmitResumeKeepsRecordedOptions(t *testing.T) {
	isolateAuthState(t)
	api := newFakeRestoreAPI(t, 0)
	api.setStatus("b@contoso.com", http.StatusTooManyRequests)
	base := []string{"restore", "submit", "--task", "64b0e9", "--date", "2026-10-01", "--entities", "a@contoso.com,b@contoso.com",
		"--concurrency", "1", "--rate", "1000", "--yes", "--json"}
	first := append(append([]string{}, base...), "--target-account", "recovery@example.com", "--include-deleted")
	if _, _, err := runAuthCmd(t, "", first...); err == nil || ExitCode(err) != 5 {
		t.Fatalf("first submit: want exit 5, got %v", err)
	}
	if body := api.lastBody("a@contoso.com"); body["targetAccount"] != "recovery@example.com" || body["includeDeleted"] != true {
		t.Fatalf("first body = %v", body)
	}
	api.setStatus("b@contoso.com", http.StatusOK)

	refusals := [][]string{
		{"--resume", "--target-account", "other@example.com"},
		{"--resume", "--include-deleted=false"},
		{"--resume", "--restore-option", "merge"},
	}
	for _, extra := range refusals {
		_, _, err := runAuthCmd(t, "", append(append([]string{}, base...), extra...)...)
		if err == nil || ExitCode(err) != 2 || !strings.Contains(err.Error(), "different options") {
			t.Fatalf("%v: want a usage refusal, got %v", extra, err)
		}
	}
	if got := api.entityPosts("b@contoso.com"); got != 1 {
		t.Fatalf("a refused resume sent b: posts = %d", got)
	}

	// Without the flags, the remaining row goes to the recorded target.
	if _, stderr, err := runAuthCmd(t, "", append(append([]string{}, base...), "--resume")...); err != nil {
		t.Fatalf("resume: %v (%s)", err, stderr)
	}
	if got := api.entityPosts("b@contoso.com"); got != 2 {
		t.Fatalf("b posts = %d, want 2", got)
	}
	if body := api.lastBody("b@contoso.com"); body["targetAccount"] != "recovery@example.com" || body["includeDeleted"] != true {
		t.Fatalf("resumed b went to %v, want the recorded recovery@example.com with includeDeleted", body)
	}

	// Repeating the recorded value is not a conflict.
	ledgerExec(t, `UPDATE restore_jobs SET status = ?, submit_status = ? WHERE entity = ?`, jobFailed, jobFailed, "b@contoso.com")
	guardReject(t, "b@contoso.com")
	same := append(append([]string{}, base...), "--resume", "--target-account", "recovery@example.com")
	if _, stderr, err := runAuthCmd(t, "", same...); err != nil {
		t.Fatalf("resume with the same target: %v (%s)", err, stderr)
	}
	if got := api.entityPosts("b@contoso.com"); got != 3 {
		t.Fatalf("b posts = %d, want 3", got)
	}

	// A task that now reports another source is refused before sending.
	ledgerExec(t, `UPDATE restore_jobs SET status = ?, submit_status = ? WHERE entity = ?`, jobFailed, jobFailed, "b@contoso.com")
	guardReject(t, "b@contoso.com")
	api.set(func(f *fakeRestoreAPI) { f.source = "GMAIL" })
	_, _, err := runAuthCmd(t, "", append(append([]string{}, base...), "--resume")...)
	if err == nil || ExitCode(err) != 2 || !strings.Contains(err.Error(), "source MS365") {
		t.Fatalf("source change: want a usage refusal, got %v", err)
	}
	if got := api.entityPosts("b@contoso.com"); got != 3 {
		t.Fatalf("source change sent b: posts = %d", got)
	}
}

// TestRestoreSubmitRerunWithoutResumeRefuses: re-running the same command
// without --resume never quietly starts a second batch over accepted rows.
func TestRestoreSubmitRerunWithoutResumeRefuses(t *testing.T) {
	isolateAuthState(t)
	api := newFakeRestoreAPI(t, 0)
	args := []string{"restore", "submit", "--task", "64b0e9", "--date", "2026-10-01", "--entities", "a@contoso.com,b@contoso.com",
		"--rate", "1000", "--yes", "--json"}
	stdout, stderr, err := runAuthCmd(t, "", args...)
	if err != nil {
		t.Fatalf("first submit: %v (%s)", err, stderr)
	}
	batchID := decodeSubmitOutput(t, stdout).BatchID

	_, _, err = runAuthCmd(t, "", args...)
	if err == nil || ExitCode(err) != 2 {
		t.Fatalf("identical re-run: want exit 2, got %v", err)
	}
	want := "cloudally-cli restore submit --date 2026-10-01 --entities a@contoso.com --entities b@contoso.com --json --rate 1000 --task 64b0e9 --resume --yes"
	if !strings.Contains(err.Error(), batchID) || !strings.Contains(err.Error(), want) || !strings.Contains(err.Error(), "--new-batch") {
		t.Fatalf("refusal should name the batch and the exact resume command %q:\n%v", want, err)
	}
	if got := api.posts.Load(); got != 2 {
		t.Fatalf("refused re-run posted: total %d, want 2", got)
	}
	if n := ledgerBatchCount(t); n != 1 {
		t.Fatalf("refused re-run recorded a batch: %d", n)
	}

	// The printed command works and sends nothing new.
	if _, stderr, err := runAuthCmd(t, "", strings.Fields(want)[1:]...); err != nil {
		t.Fatalf("printed resume command: %v (%s)", err, stderr)
	}
	if got := api.posts.Load(); got != 2 {
		t.Fatalf("printed resume command re-sent: total %d", got)
	}

	if _, _, err := runAuthCmd(t, "", append(args, "--new-batch", "--resume")...); err == nil || ExitCode(err) != 2 {
		t.Fatalf("--new-batch with --resume: want exit 2, got %v", err)
	}
	if _, stderr, err := runAuthCmd(t, "", append(args, "--new-batch")...); err != nil {
		t.Fatalf("--new-batch: %v (%s)", err, stderr)
	}
	if got := api.posts.Load(); got != 4 {
		t.Fatalf("--new-batch total posts %d, want 4", got)
	}
	if n := ledgerBatchCount(t); n != 2 {
		t.Fatalf("--new-batch batches = %d, want 2", n)
	}
}

// TestRestoreSubmitRerunAfterOnlyRejectionsIsAllowed is the other
// direction: a batch whose every row was rejected before it ran holds
// nothing the API accepted, so the same command may start a new batch.
func TestRestoreSubmitRerunAfterOnlyRejectionsIsAllowed(t *testing.T) {
	isolateAuthState(t)
	api := newFakeRestoreAPI(t, http.StatusBadRequest)
	args := []string{"restore", "submit", "--task", "64b0e9", "--date", "2026-10-01", "--entities", "a@contoso.com,b@contoso.com",
		"--rate", "1000", "--yes", "--json"}
	if _, _, err := runAuthCmd(t, "", args...); err == nil || ExitCode(err) != 5 {
		t.Fatalf("first submit: want exit 5, got %v", err)
	}
	api.set(func(f *fakeRestoreAPI) { f.restoreErr = 0 })
	if _, stderr, err := runAuthCmd(t, "", args...); err != nil {
		t.Fatalf("re-run after rejections only: %v (%s)", err, stderr)
	}
	if got := api.posts.Load(); got != 4 {
		t.Fatalf("total posts %d, want 4", got)
	}
}

// TestRestoreUnknownNoticePrintsWorkingCommand: the notice names a command
// that runs as printed and sends exactly the UNKNOWN row.
func TestRestoreUnknownNoticePrintsWorkingCommand(t *testing.T) {
	isolateAuthState(t)
	api := newFakeRestoreAPI(t, 0)
	api.setStatus("b@contoso.com", http.StatusInternalServerError)
	args := []string{"restore", "submit", "--task", "64b0e9", "--date", "2026-10-01", "--entities", "a@contoso.com,b@contoso.com",
		"--concurrency", "1", "--rate", "1000", "--yes", "--json"}
	_, stderr, err := runAuthCmd(t, "", args...)
	if err == nil || ExitCode(err) != 5 {
		t.Fatalf("submit: want exit 5, got %v", err)
	}
	const marker = "send the ones still UNKNOWN with: "
	i := strings.Index(stderr, marker)
	if i < 0 {
		t.Fatalf("notice has no command:\n%s", stderr)
	}
	line := strings.TrimSpace(strings.SplitN(stderr[i+len(marker):], "\n", 2)[0])
	fields := strings.Fields(line)
	if len(fields) < 3 || fields[0] != "cloudally-cli" || !strings.HasSuffix(line, "--resume --retry-unknown --yes") {
		t.Fatalf("printed command = %q", line)
	}
	api.setStatus("b@contoso.com", http.StatusOK)
	if _, stderr, err := runAuthCmd(t, "", fields[1:]...); err != nil {
		t.Fatalf("printed command %q failed: %v (%s)", line, err, stderr)
	}
	if api.entityPosts("b@contoso.com") != 2 || api.entityPosts("a@contoso.com") != 1 {
		t.Fatalf("per-entity posts = %v", api.byEntity)
	}
}

const evilEntity = "x\x1b]0;PWNED\x07\x1b[2J\u009b31m\u202egnp.exe\u2066@contoso.com"

func assertTerminalSafe(t *testing.T, what, s string) {
	t.Helper()
	for _, bad := range []string{"\x1b", "\x07", "\u009b", "\u202e", "\u2066"} {
		if strings.Contains(s, bad) {
			t.Fatalf("%s carries %q:\n%q", what, bad, s)
		}
	}
}

func TestTermSafe(t *testing.T) {
	got := termSafe("a\tb\nc\x1b[2J\x07\u0085\u009b\u202a\u202e\u2066\u2069d\x7f")
	if got != "a b c[2Jd" {
		t.Fatalf("termSafe = %q", got)
	}
	if termSafe("jane.doe@contoso.com") != "jane.doe@contoso.com" || termSafe("\u05d0\u05d1@contoso.com") != "\u05d0\u05d1@contoso.com" {
		t.Fatal("termSafe must leave ordinary text alone")
	}
	var buf bytes.Buffer
	if err := printRestoreTable(&buf, []string{"en\x1btity", "status"}, [][]string{{evilEntity, "OK\nFAKE ROW"}}); err != nil {
		t.Fatal(err)
	}
	assertTerminalSafe(t, "table", buf.String())
	if n := strings.Count(buf.String(), "\n"); n != 2 {
		t.Fatalf("a newline in a cell must not add a row: %d lines\n%s", n, buf.String())
	}
}

// TestRestoreHumanOutputNeutralisesTenantControls drives the reviewer's
// case: a mailbox name carrying OSC, CSI, BEL, C1, and bidi controls in
// 'restore plan --human-friendly' and in the submit table and notices.
func TestRestoreHumanOutputNeutralisesTenantControls(t *testing.T) {
	isolateAuthState(t)
	api := newFakeRestoreAPI(t, 0)
	api.set(func(f *fakeRestoreAPI) { f.entities = []string{evilEntity, "a@contoso.com"} })
	stdout, stderr, err := runAuthCmd(t, "", "restore", "plan", "--task", "64b0e9", "--date", "2026-10-01", "--human-friendly")
	if err != nil {
		t.Fatalf("plan: %v (%s)", err, stderr)
	}
	if !strings.Contains(stdout, "PWNED") {
		t.Fatalf("plan should still list the mailbox:\n%s", stdout)
	}
	assertTerminalSafe(t, "plan stdout", stdout)

	api.setStatus(evilEntity, http.StatusInternalServerError)
	stdout, stderr, err = runAuthCmd(t, "", "restore", "submit", "--task", "64b0e9", "--date", "2026-10-01", "--rate", "1000", "--yes", "--human-friendly")
	if err == nil || ExitCode(err) != 5 {
		t.Fatalf("submit: want exit 5, got %v", err)
	}
	if !strings.Contains(stderr, "UNKNOWN") {
		t.Fatalf("stderr should list the UNKNOWN row:\n%s", stderr)
	}
	assertTerminalSafe(t, "submit stdout", stdout)
	assertTerminalSafe(t, "submit stderr", stderr)

	batchID := ""
	for _, j := range ledgerRows(t) {
		batchID = j.BatchID
	}
	stdout, _, _ = runAuthCmd(t, "", "restore", "history", "--batch", batchID, "--human-friendly")
	assertTerminalSafe(t, "history stdout", stdout)
	stdout, stderr, _ = runAuthCmd(t, "", "restore", "watch", "--batch", batchID, "--once", "--human-friendly")
	assertTerminalSafe(t, "watch stdout", stdout)
	assertTerminalSafe(t, "watch stderr", stderr)
}
