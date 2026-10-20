// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cloudally-pp-cli/internal/cliutil/testenv"
	"cloudally-pp-cli/internal/portal"
)

// compactAt renders a time as the portal's compact yyyyMMddHHmmss.
func compactAt(t time.Time) int64 {
	var v int64
	for _, c := range t.UTC().Format("20060102150405") {
		v = v*10 + int64(c-'0')
	}
	return v
}

// TestMatchRowNeedsProofTheJobIsTheRows covers which portal job counts as
// a row's archive when several share its mailbox, sub-source and snapshot.
func TestMatchRowNeedsProofTheJobIsTheRows(t *testing.T) {
	requested := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	row := downloadRow{Entity: "a@contoso.com", SubSource: "O365MAIL", SnapshotRaw: 20221231042000,
		Label: rowLabelPrefix + "mine", RequestedAt: isoUTC(requested)}
	job := func(id, label string, created time.Time, files ...string) portal.DownloadJob {
		j := portal.DownloadJob{JobID: id, Account: "a@contoso.com", SubSource: "O365MAIL", SnapshotDate: 20221231042000, JobLabel: label}
		if !created.IsZero() {
			j.InitDate = compactAt(created)
		}
		for _, f := range files {
			j.Chunks = append(j.Chunks, portal.DownloadFile{FileName: f})
		}
		return j
	}
	older := requested.Add(-48 * time.Hour)
	later := requested.Add(2 * time.Minute)
	foreignLabel := rowLabelPrefix + "other-archive"
	cases := []struct {
		name          string
		jobs          []portal.DownloadJob
		format        string
		wantID        string
		wantAmbiguous bool
		labelsAreKept bool
	}{
		{name: "its own label", jobs: []portal.DownloadJob{job("old", "", older), job("mine", rowLabelPrefix+"mine", later)}, wantID: "mine"},
		{name: "a label the guard records for another archive is never it", jobs: []portal.DownloadJob{job("theirs", foreignLabel, later)}},
		{name: "a different label could be its own, edited", jobs: []portal.DownloadJob{job("edited", "SRV#6840424", later)}, wantAmbiguous: true},
		{name: "labels kept, its label lost: could be it", jobs: []portal.DownloadJob{job("lost", "", later)}, labelsAreKept: true, wantAmbiguous: true},
		{name: "an export made days before the request is not it", jobs: []portal.DownloadJob{job("old", "", older)}},
		{name: "a job made minutes before the request is never accepted", jobs: []portal.DownloadJob{job("early", "", requested.Add(-5*time.Minute))}, wantAmbiguous: true},
		{name: "a pst export is not a zip", jobs: []portal.DownloadJob{job("pst", "", later, "a.pst", "b.pst")}, format: downloadFormatZip},
		{name: "one unlabelled job made after the request", jobs: []portal.DownloadJob{job("old", "", older), job("new", "", later)}, wantID: "new"},
		{name: "two such jobs prove nothing", jobs: []portal.DownloadJob{job("n1", "", later), job("n2", "", later)}, wantAmbiguous: true},
		{name: "no creation time proves nothing", jobs: []portal.DownloadJob{job("n1", "", time.Time{})}, wantAmbiguous: true},
	}
	foreign := func(label string) bool { return label == foreignLabel }
	for _, tc := range cases {
		r := row
		format := tc.format
		if format == "" {
			format = downloadFormatPST
		}
		m := matchRow(&r, tc.jobs, format, tc.labelsAreKept, foreign)
		switch {
		case tc.wantID != "":
			if !m.found || m.job.JobID != tc.wantID {
				t.Fatalf("%s: want job %s, got %+v", tc.name, tc.wantID, m)
			}
		case tc.wantAmbiguous:
			if m.found || !m.ambiguous {
				t.Fatalf("%s: want ambiguous, got %+v", tc.name, m)
			}
		default:
			if m.found || m.ambiguous {
				t.Fatalf("%s: want no match, got %+v", tc.name, m)
			}
		}
	}
}

// TestReconcileNeverUnconfirmsACreatedRow proves a job confirmed once stays
// created when it later drops out of the listing (as an empty job does when
// it finishes), so no resume creates it again.
func TestReconcileNeverUnconfirmsACreatedRow(t *testing.T) {
	testenv.Isolate(t)
	prevDelay := reconcileDelay
	reconcileDelay = time.Millisecond
	t.Cleanup(func() { reconcileDelay = prevDelay })
	s := newDLStub(t)
	b := newSubmitBatch(t, "20221231-000000-kept", "a@contoso.com", "b@contoso.com")
	b.Rows[0].Status, b.Rows[0].JobID, b.Rows[0].ConfirmedAt = dlCreated, "job-a", isoUTC(time.Now())
	b.Rows[1].Status = dlFailed
	if err := saveBatch(b); err != nil {
		t.Fatal(err)
	}
	// job-a has finished empty and is no longer listed. Two resumes: the
	// first re-sends b@ and reconciles, the second must send nothing.
	for i := 0; i < 2; i++ {
		_, _, _ = runDL(t, s, "download", "submit", "--batch", b.BatchKey, "--resume", "--yes", "--json")
	}
	for _, a := range s.createdAccounts() {
		if a == "a@contoso.com" {
			t.Fatalf("a confirmed export was created again: %v", s.createdAccounts())
		}
	}
	after, _ := loadBatch(b.BatchKey)
	if r := rowByEntity(after, "a@contoso.com"); r.Status != dlCreated || r.JobID != "job-a" {
		t.Fatalf("a confirmed row must stay CREATED: %+v", r)
	}
}

// TestRetryUnknownIgnoresAnOlderExportOfTheSameSnapshot is the legal-hold
// case: an older zip of the same snapshot must not confirm a PST
// legal-hold row, and fetch --batch must not take it as the batch's.
func TestRetryUnknownIgnoresAnOlderExportOfTheSameSnapshot(t *testing.T) {
	testenv.Isolate(t)
	s := newDLStub(t)
	b := newSubmitBatch(t, "20221231-000000-hold", jane)
	yes := true
	b.Format, b.IncludeLegalHold = downloadFormatPST, &yes
	b.Rows[0].Status, b.Rows[0].Label, b.Rows[0].RequestedAt = dlUnknown, rowLabelPrefix+"hold1", isoUTC(time.Now().Add(-time.Hour))
	if err := saveBatch(b); err != nil {
		t.Fatal(err)
	}
	// Last year's zip of the same snapshot, made by someone else.
	s.addFile("j-old", jane, "O365MAIL", "d-old", "jane.zip", []byte("PK\x03\x04old zip"))
	s.mu.Lock()
	s.jobs[0].Snapshot = b.Rows[0].SnapshotRaw
	s.jobs[0].Created = compactAt(time.Now().AddDate(-1, 0, 0))
	s.listCreated = true
	s.mu.Unlock()

	// --no-reconcile leaves the re-sent row REQUESTED, which is unsettled.
	if _, stderr, err := runDL(t, s, "download", "submit", "--batch", b.BatchKey, "--resume", "--retry-unknown", "--no-reconcile", "--yes"); err == nil || ExitCode(err) != 6 {
		t.Fatalf("resume: want exit 6 for an unconfirmed row, got %v\n%s", err, stderr)
	}
	if got := s.createdAccounts(); len(got) != 1 {
		t.Fatalf("the older zip must not stand in for the row; want it sent once, got %v", got)
	}
	after, _ := loadBatch(b.BatchKey)
	if r := rowByEntity(after, jane); r.JobID == "j-old" {
		t.Fatalf("the row was confirmed by the older export: %+v", r)
	}

	// Confirm the row against its own new job, then fetch the batch: only
	// that job is downloaded.
	s.mu.Lock()
	newJob := &s.jobs[len(s.jobs)-1]
	newJob.Status = portal.StatusReadyForDownload
	newJob.Files = []portal.DownloadFile{{ExportDataID: "d-new", FileName: "jane.pst", Size: "9 Bytes"}}
	s.bodies["d-new"] = []byte("!BDNpst!!")
	s.mu.Unlock()
	if _, stderr, err := runDL(t, s, "download", "submit", "--batch", b.BatchKey, "--resume", "--yes"); err != nil {
		t.Fatalf("confirming resume: %v\n%s", err, stderr)
	}
	out, stderr, err := runDL(t, s, fetchArgs(t.TempDir(), "--batch", b.BatchKey)...)
	if err != nil {
		t.Fatalf("fetch --batch: %v\n%s", err, stderr)
	}
	for _, o := range decodeOutcomes(t, out) {
		if o.JobID == "j-old" {
			t.Fatalf("fetch --batch took the older export: %+v", o)
		}
	}
	if s.hits("d-old") != 0 || s.hits("d-new") != 1 {
		t.Fatalf("hits: old=%d new=%d", s.hits("d-old"), s.hits("d-new"))
	}
}

// writeLegacyLedger writes a ledger the way the first build did: no
// rerun_key, no include flags, no labels.
func writeLegacyLedger(t *testing.T, key string, rows []downloadRow) {
	t.Helper()
	b := map[string]any{
		"batch_key": key, "scope": restoreScope{account: "acct-1"}.kind(), "account_id": "acct-1",
		"task_id": "task-1", "source": "MS365", "on_date": latestLabel, "timezone": "UTC", "format": "zip",
		"created_at": "2026-10-15T20:15:00Z", "rows": rows,
	}
	raw, _ := json.MarshalIndent(b, "", "  ")
	path, err := batchPath(key)
	if err != nil {
		t.Fatal(err)
	}
	mustMkdir(t, filepath.Dir(path))
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestLegacyLedgersStillGuardAndResumeSafely covers ledgers from the first
// build: they still block a run that would create their exports again,
// and their resume refusal never sends the operator to --new-batch.
func TestLegacyLedgersStillGuardAndResumeSafely(t *testing.T) {
	testenv.Isolate(t)
	s := newDLStub(t)
	quickReconcile(t, s)
	s.taskEntities = []string{"a@contoso.com", "b@contoso.com"}
	const key = "20261015-201500-acct"
	writeLegacyLedger(t, key, []downloadRow{
		{Entity: "a@contoso.com", SubSource: "O365MAIL", SnapshotRaw: 20221231042000, Status: dlCreated, JobID: "job-a"},
		// Its error says the connection was refused, but that build wrote
		// FAILED for every error, so it is held as possibly created.
		{Entity: "a@contoso.com", SubSource: "O365CALENDAR", SnapshotRaw: 20221231041000, Status: dlFailed,
			Detail: "portal: POST startRestoreOrDownload: dial tcp 10.0.0.1:443: connect: connection refused"},
		{Entity: "b@contoso.com", SubSource: "O365MAIL", SnapshotRaw: 20221231042001, Status: dlPlanned},
		{Entity: "b@contoso.com", SubSource: "O365CALENDAR", SnapshotRaw: 20221231041001, Status: dlRequested},
	})

	_, stderr0, err := runDL(t, s, downloadSubmitArgs("--no-reconcile")...)
	if err == nil || ExitCode(err) != 2 || !strings.Contains(stderr0, "batch "+key+" holds") {
		t.Fatalf("the same submit after an upgrade must be refused by the old ledger, got %v\n%s", err, stderr0)
	}
	if strings.Contains(err.Error()+stderr0, "--new-batch") {
		t.Fatalf("the refusal must not suggest --new-batch:\n%v\n%s", err, stderr0)
	}
	if n := len(s.createdAccounts()); n != 0 {
		t.Fatalf("a refused run created %d export(s)", n)
	}

	_, stderr, err := runDL(t, s, "download", "submit", "--batch", key, "--resume", "--yes")
	if err == nil || ExitCode(err) != 2 {
		t.Fatalf("resuming an old ledger without its include flags must be refused, got %v", err)
	}
	if strings.Contains(err.Error()+stderr, "--new-batch") {
		t.Fatalf("the refusal must not suggest --new-batch:\n%v\n%s", err, stderr)
	}
	for _, want := range []string{
		"--resume --include-deleted=false --include-legal-hold=false --yes",
		"--latest --entities b@contoso.com --sub-sources O365MAIL --yes",
	} {
		if !strings.Contains(err.Error()+stderr, want) {
			t.Fatalf("the refusal should offer %q:\n%v\n%s", want, err, stderr)
		}
	}
	if strings.Contains(stderr, "--entities a@contoso.com") || strings.Contains(stderr, "--entities b@contoso.com --sub-sources O365CALENDAR,") || strings.Contains(stderr, "O365MAIL,O365CALENDAR") {
		t.Fatalf("a row that may exist must not be offered:\n%s", stderr)
	}

	// One of the offered commands goes through and creates only its row.
	if _, stderr, err := runDL(t, s, "download", "submit", "--account", "acct-1", "--task", "task-1", "--latest",
		"--entities", "b@contoso.com", "--sub-sources", "O365MAIL", "--yes"); err != nil {
		t.Fatalf("an offered command was refused: %v\n%s", err, stderr)
	}
	if got := s.createdAccounts(); len(got) != 1 || got[0] != "b@contoso.com" {
		t.Fatalf("the offered command should create exactly b@'s mail, got %v", got)
	}

	// Stating the include flags resumes the old batch and records them.
	if _, stderr, err := runDL(t, s, "download", "submit", "--batch", key, "--resume", "--include-deleted=false", "--include-legal-hold=false", "--yes", "--no-reconcile"); err != nil && ExitCode(err) != 6 {
		t.Fatalf("resume with stated flags: %v\n%s", err, stderr)
	}
	after, err := loadBatch(key)
	if err != nil || after.IncludeDeleted == nil || after.IncludeLegalHold == nil || *after.IncludeDeleted || *after.IncludeLegalHold {
		t.Fatalf("the stated flags should be recorded: %+v %v", after, err)
	}
	// b@'s mail was created by the offered command, so the resume left it
	// out; a@'s calendar may exist, so it is UNKNOWN and not sent.
	if got := s.createdAccounts(); len(got) != 1 {
		t.Fatalf("want only b@ mail, once, got %v", got)
	}
	if r := after.Rows[2]; r.Status != dlSkipped || r.HeldBy == "" || r.HeldBy == key {
		t.Fatalf("b@'s mail should be SKIPPED in the old batch: %+v", r)
	}
	if r := after.Rows[1]; r.Status != dlUnknown {
		t.Fatalf("a@'s legacy FAILED calendar should be held as UNKNOWN: %+v", r)
	}
}

// TestRerunGuardFailsClosedOnAnUnreadableLedger proves a ledger the guard
// cannot read blocks a new batch instead of being skipped.
func TestRerunGuardFailsClosedOnAnUnreadableLedger(t *testing.T) {
	testenv.Isolate(t)
	s := newDLStub(t)
	s.taskEntities = []string{"a@contoso.com"}
	path, _ := batchPath("20261015-201500-broken")
	mustMkdir(t, filepath.Dir(path))
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, _, err := runDL(t, s, downloadSubmitArgs("--no-reconcile")...)
	if err == nil || ExitCode(err) != 2 || !strings.Contains(err.Error(), "20261015-201500-broken.json") {
		t.Fatalf("an unreadable ledger must block with exit 2 and be named, got %v", err)
	}
	if n := len(s.createdAccounts()); n != 0 {
		t.Fatalf("created %d export(s) past an unreadable ledger", n)
	}
}

// TestDownloadFetchExitsNonZeroWhenNothingMatched covers a run that finds no
// job at all, such as a wrong --on-date.
func TestDownloadFetchExitsNonZeroWhenNothingMatched(t *testing.T) {
	testenv.Isolate(t)
	s := newDLStub(t)
	s.addFile("j-mail", jane, "O365MAIL", "d1", "jane.zip", []byte("PK\x03\x04mail"))
	_, _, err := runDL(t, s, fetchArgs(t.TempDir(), "--on-date", "2021-12-31")...)
	if err == nil || ExitCode(err) != 3 || !strings.Contains(err.Error(), "no download jobs matched") {
		t.Fatalf("want exit 3 with 'no download jobs matched', got %v", err)
	}
}

// TestDownloadWatchExitCodes covers --once and a finished job.
func TestDownloadWatchExitCodes(t *testing.T) {
	testenv.Isolate(t)
	s := newDLStub(t)
	s.addFile("job-1", jane, "O365MAIL", "d1", "jane.zip", []byte("PK\x03\x04"))
	watch := func(extra ...string) error {
		_, _, err := runDL(t, s, append([]string{"download", "watch", "--account", "acct-1", "--job", "job-1", "--json"}, extra...)...)
		return err
	}
	for _, tc := range []struct {
		status string
		once   bool
		want   int
	}{
		{portal.StatusReadyForDownload, true, 0},
		{"INPROCESS", true, ExitJobPending},
		{"FAILED", true, 6},
		{"CANCELLED", true, 6},
		{"FAILED", false, 6},
	} {
		s.mu.Lock()
		s.jobs[0].Status = tc.status
		s.mu.Unlock()
		var err error
		if tc.once {
			err = watch("--once")
		} else {
			err = watch()
		}
		got := 0
		if err != nil {
			got = ExitCode(err)
		}
		if got != tc.want {
			t.Fatalf("watch on %s (once=%v): exit %d, want %d (%v)", tc.status, tc.once, got, tc.want, err)
		}
	}
}
