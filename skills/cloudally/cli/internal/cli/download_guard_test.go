// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"cloudally-pp-cli/internal/cliutil/testenv"
	"cloudally-pp-cli/internal/portal"
)

// guardStates reads every claim's state in the download guard, by
// entity/sub-source/format.
func guardStates(t *testing.T) map[string][]string {
	t.Helper()
	g := testGuard(t)
	rows, err := g.db.QueryContext(context.Background(), `SELECT entity, sub_source, format, include_legal_hold, state, superseded FROM download_claims ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[string][]string{}
	for rows.Next() {
		var entity, sub, format, state string
		var hold, superseded int
		if err := rows.Scan(&entity, &sub, &format, &hold, &state, &superseded); err != nil {
			t.Fatal(err)
		}
		k := entity + "/" + sub + "/" + format
		if hold == 1 {
			k += "/hold"
		}
		if superseded == 1 {
			state += "(superseded)"
		}
		out[k] = append(out[k], state)
	}
	return out
}

// Item 1: a different format or include flags is a different archive.
func TestDownloadGuardIdentityIncludesFormatAndFlags(t *testing.T) {
	testenv.Isolate(t)
	s := newDLStub(t)
	quickReconcile(t, s)
	s.taskEntities = []string{"a@contoso.com"}
	for i, extra := range [][]string{nil, {"--format", "pst"}, {"--include-legal-hold"}} {
		_, stderr, err := runDL(t, s, downloadSubmitArgs(extra...)...)
		if err != nil {
			t.Fatalf("run %d %v: %v\n%s", i, extra, err, stderr)
		}
		if n := len(s.createdAccounts()); n != 2*(i+1) {
			t.Fatalf("run %d %v: creates = %d, want %d (nothing SKIPPED)", i, extra, n, 2*(i+1))
		}
	}
	for _, k := range ledgerKeys(t) {
		b, _ := loadBatch(k)
		for _, r := range b.Rows {
			if r.Status != dlCreated {
				t.Fatalf("batch %s (%s): row %s/%s is %s, want CREATED", k, b.Format, r.Entity, r.SubSource, r.Status)
			}
		}
	}
	states := guardStates(t)
	for _, k := range []string{"a@contoso.com/O365MAIL/zip", "a@contoso.com/O365MAIL/pst", "a@contoso.com/O365MAIL/zip/hold"} {
		if got := states[k]; len(got) != 1 || got[0] != dgCreated {
			t.Fatalf("guard claim %s = %v, want one CREATED", k, got)
		}
	}
	// SKIPPED, FAILED and JOB_FAILED rows are never a batch's own.
	b := &downloadBatch{Rows: []downloadRow{{Status: dlSkipped}, {Status: dlFailed}, {Status: dlJobFailed}}}
	if n := ownedRows(b); n != 0 {
		t.Fatalf("ownedRows = %d, want 0", n)
	}
	for _, st := range []string{dlSkipped, dlJobFailed} {
		if mayExist(downloadRow{Status: st, ConfirmedAt: "x", AcceptedAt: "x"}) {
			t.Fatalf("%s must not count as an archive that may exist", st)
		}
	}
}

// Item 2: a job of the same mailbox, sub-source and snapshot whose label
// was edited makes the row ambiguous, never EMPTY or MISSING.
func TestDownloadEditedLabelIsAmbiguousNotEmpty(t *testing.T) {
	testenv.Isolate(t)
	s := newDLStub(t)
	quickReconcile(t, s)
	s.taskEntities = []string{"a@contoso.com"}
	s.countCreates = true // the dashboard accounts for every create
	s.relabel = func(_, sub, label string) string {
		if sub == "O365CALENDAR" {
			return "SRV#99" // an operator edited this job's label
		}
		return label
	}
	_, stderr, err := runDL(t, s, downloadSubmitArgs()...)
	if err == nil || ExitCode(err) != 6 || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("want exit 6 for an ambiguous row, got %v\n%s", err, stderr)
	}
	if !strings.Contains(stderr, "download fetch --account acct-1 --dest <folder> --year <year> --on-date 2022-12-31 --entities a@contoso.com") {
		t.Fatalf("the snapshot-wide fetch should be printed:\n%s", stderr)
	}
	b, _ := loadBatch(ledgerKeys(t)[0])
	for _, r := range b.Rows {
		if r.SubSource == "O365CALENDAR" && (r.Status != dlUnknown || !r.Ambiguous) {
			t.Fatalf("the edited-label row must be ambiguous UNKNOWN, got %+v", r)
		}
	}
	// A resume never sends it again.
	before := len(s.createdAccounts())
	_, _, _ = runDL(t, s, "download", "submit", "--batch", b.BatchKey, "--resume", "--retry-unknown", "--yes")
	if n := len(s.createdAccounts()); n != before {
		t.Fatalf("an ambiguous row was sent again: %d creates", n-before)
	}
}

// Item 3: fetch hands a failed job back to its batch, which creates it
// exactly once more and can then finish.
func TestDownloadFailedJobIsCreatedOnceMoreByItsBatch(t *testing.T) {
	testenv.Isolate(t)
	s := newDLStub(t)
	quickReconcile(t, s)
	s.taskEntities = []string{"a@contoso.com"}
	if _, stderr, err := runDL(t, s, downloadSubmitArgs("--sub-sources", "O365MAIL")...); err != nil {
		t.Fatalf("submit: %v\n%s", err, stderr)
	}
	key := ledgerKeys(t)[0]
	s.mu.Lock()
	s.jobs[0].Status = "FAILED"
	s.mu.Unlock()

	out, stderr, err := runDL(t, s, fetchArgs(t.TempDir(), "--batch", key)...)
	if err == nil || ExitCode(err) != 6 {
		t.Fatalf("fetch of a failed job: want exit 6, got %v", err)
	}
	if strings.Contains(out+stderr, "--new-batch") {
		t.Fatalf("fetch must not suggest --new-batch:\n%s\n%s", out, stderr)
	}
	if !strings.Contains(out, "--batch "+key+" --resume --yes") {
		t.Fatalf("fetch should name the batch's resume:\n%s", out)
	}
	b, _ := loadBatch(key)
	if b.Rows[0].Status != dlJobFailed {
		t.Fatalf("fetch should mark the row JOB_FAILED, got %+v", b.Rows[0])
	}
	if got := guardStates(t)["a@contoso.com/O365MAIL/zip"]; len(got) != 1 || got[0] != dgJobFailed {
		t.Fatalf("guard claim = %v, want JOB_FAILED", got)
	}

	for i := 0; i < 3; i++ {
		_, _, _ = runDL(t, s, "download", "submit", "--batch", key, "--resume", "--yes")
	}
	if n := len(s.createdAccounts()); n != 2 {
		t.Fatalf("creates = %d after three resumes, want 2 (the original and one re-create)", n)
	}
	// The new job becomes ready: the batch finishes.
	s.mu.Lock()
	j := &s.jobs[len(s.jobs)-1]
	j.Status = portal.StatusReadyForDownload
	j.Files = []portal.DownloadFile{{ExportDataID: "d-new", FileName: "a.zip", Size: "6 Bytes"}}
	s.bodies["d-new"] = []byte("PK\x03\x04ok")
	s.mu.Unlock()
	if _, stderr, err := runDL(t, s, fetchArgs(t.TempDir(), "--batch", key)...); err != nil {
		t.Fatalf("fetch after the re-create: %v\n%s", err, stderr)
	}
}

// Item 3, the resume side: a resume finds a failed confirmed job itself.
func TestDownloadResumeFindsAFailedJob(t *testing.T) {
	testenv.Isolate(t)
	s := newDLStub(t)
	quickReconcile(t, s)
	s.taskEntities = []string{"a@contoso.com"}
	if _, stderr, err := runDL(t, s, downloadSubmitArgs("--sub-sources", "O365MAIL")...); err != nil {
		t.Fatalf("submit: %v\n%s", err, stderr)
	}
	s.mu.Lock()
	s.jobs[0].Status = "CANCELLED"
	s.mu.Unlock()
	if _, stderr, err := runDL(t, s, "download", "submit", "--batch", ledgerKeys(t)[0], "--resume", "--yes"); err != nil {
		t.Fatalf("resume: %v\n%s", err, stderr)
	}
	if n := len(s.createdAccounts()); n != 2 {
		t.Fatalf("creates = %d, want 2: the cancelled job is created once more", n)
	}
}

// Item 4: two batches with different, overlapping selections race; every
// archive is created once.
func TestDownloadOverlappingBatchesRaceAcrossProcesses(t *testing.T) {
	if testing.Short() {
		t.Skip("starts child processes")
	}
	testenv.Isolate(t)
	s := newDLStub(t)
	quickReconcile(t, s)
	s.mu.Lock()
	s.taskEntities = []string{"a@contoso.com", "b@contoso.com", "c@contoso.com"}
	s.mu.Unlock()
	res := runChildrenClaimPause(t, s, 700*time.Millisecond,
		downloadSubmitArgs("--entities", "a@contoso.com,b@contoso.com"),
		downloadSubmitArgs("--entities", "b@contoso.com,c@contoso.com"))
	for _, r := range res {
		if r.code != 0 {
			t.Fatalf("both runs should settle (the loser's shared rows are held by the winner): %+v", res)
		}
	}
	seen := map[string]int{}
	s.mu.Lock()
	for _, body := range s.createBodies {
		seen[body["account"].(string)+"/"+body["source"].(string)]++
	}
	s.mu.Unlock()
	if len(seen) != 6 {
		t.Fatalf("want 6 distinct archives, got %v", seen)
	}
	for k, n := range seen {
		if n != 1 {
			t.Fatalf("%s was created %d times", k, n)
		}
	}
}

// runChildrenClaimPause starts the helper processes together, each pausing
// after its up-front check and before claiming.
func runChildrenClaimPause(t *testing.T, s *dlStub, pause time.Duration, argLists ...[]string) []childResult {
	t.Helper()
	t.Setenv(helperClaim, pause.String())
	return runChildren(t, s, 0, 0, argLists...)
}

// Item 5: a confirmed row keeps its pinned job, even when that job is not
// listed and an unrelated job of the same snapshot is.
func TestDownloadConfirmedRowIsNeverReMatched(t *testing.T) {
	testenv.Isolate(t)
	s := newDLStub(t)
	quickReconcile(t, s)
	b := newSubmitBatch(t, "20221231-000000-pinned", "a@contoso.com", "b@contoso.com")
	b.Rows[0].Status, b.Rows[0].JobID, b.Rows[0].Label = dlCreated, "job-pinned", rowLabelPrefix+"pinned"
	b.Rows[0].RequestedAt = isoUTC(time.Now().Add(-time.Hour))
	b.Rows[1].Status = dlFailed
	if err := saveBatch(b); err != nil {
		t.Fatal(err)
	}
	s.jobs = append(s.jobs, dlJob{JobID: "job-unrelated", Account: "a@contoso.com", SubSource: "O365MAIL", Status: "INPROCESS",
		Snapshot: b.Rows[0].SnapshotRaw, Created: compactNow()})
	_, _, _ = runDL(t, s, "download", "submit", "--batch", b.BatchKey, "--resume", "--yes")
	if r := rowByEntity(mustLoadBatch(t, b.BatchKey), "a@contoso.com"); r.Status != dlCreated || r.JobID != "job-pinned" {
		t.Fatalf("a confirmed row was re-matched: %+v", r)
	}
}

// Item 7: no refusal or advice suggests --new-batch.
func TestDownloadResumeConflictDoesNotSuggestNewBatch(t *testing.T) {
	testenv.Isolate(t)
	s := newDLStub(t)
	b := newSubmitBatch(t, "20221231-000000-conflict", "a@contoso.com")
	_, stderr, err := runDL(t, s, "download", "submit", "--batch", b.BatchKey, "--resume", "--include-legal-hold", "--yes")
	if err == nil || ExitCode(err) != 2 {
		t.Fatalf("a conflicting flag must be refused, got %v", err)
	}
	if strings.Contains(err.Error()+stderr, "--new-batch") {
		t.Fatalf("the refusal must not suggest --new-batch:\n%v\n%s", err, stderr)
	}
}

// Item 8: a row never confirmed keeps the batch unsettled.
func TestDownloadUnconfirmedRowsAreUnsettled(t *testing.T) {
	testenv.Isolate(t)
	s := newDLStub(t)
	s.taskEntities = []string{"a@contoso.com"}
	_, _, err := runDL(t, s, downloadSubmitArgs("--no-reconcile")...)
	if err == nil || ExitCode(err) != 6 || !strings.Contains(err.Error(), "REQUESTED and not yet confirmed") {
		t.Fatalf("unconfirmed rows must exit 6, got %v", err)
	}
}

// Item 9: a legacy ledger's recorded format is checked on resume too.
func TestDownloadLegacyResumeChecksFormat(t *testing.T) {
	testenv.Isolate(t)
	s := newDLStub(t)
	const key = "20261015-201500-fmt"
	writeLegacyLedger(t, key, []downloadRow{{Entity: "a@contoso.com", SubSource: "O365MAIL", SnapshotRaw: 20221231042000, Status: dlFailed}})
	_, _, err := runDL(t, s, "download", "submit", "--batch", key, "--resume", "--format", "pst",
		"--include-deleted=false", "--include-legal-hold=false", "--yes")
	if err == nil || ExitCode(err) != 2 || !strings.Contains(err.Error(), "--format pst (batch recorded zip)") {
		t.Fatalf("a legacy resume with another format must be refused, got %v", err)
	}
	if n := len(s.createdAccounts()); n != 0 {
		t.Fatalf("created %d", n)
	}
}

// A portal that does not keep job labels: rows are confirmed by creation
// time, one job each, and the batch finishes.
func TestDownloadPortalWithoutLabels(t *testing.T) {
	testenv.Isolate(t)
	s := newDLStub(t)
	quickReconcile(t, s)
	s.taskEntities = []string{"a@contoso.com"}
	s.relabel = func(_, _, _ string) string { return "" }
	if _, stderr, err := runDL(t, s, downloadSubmitArgs()...); err != nil {
		t.Fatalf("submit: %v\n%s", err, stderr)
	}
	b := mustLoadBatch(t, ledgerKeys(t)[0])
	ids := map[string]bool{}
	for _, r := range b.Rows {
		if r.Status != dlCreated || r.JobID == "" || ids[r.JobID] {
			t.Fatalf("every row should be confirmed to its own job: %+v", b.Rows)
		}
		ids[r.JobID] = true
	}
	if got := guardStates(t)["a@contoso.com/O365MAIL/zip"]; len(got) != 1 || got[0] != dgCreated {
		t.Fatalf("guard = %v", got)
	}
	var pinned string
	g := testGuard(t)
	if err := g.db.QueryRow(`SELECT job_id FROM download_claims WHERE sub_source = 'O365MAIL'`).Scan(&pinned); err != nil && err != sql.ErrNoRows {
		t.Fatal(err)
	}
	if pinned == "" {
		t.Fatal("the guard should pin the confirmed job id")
	}
}
