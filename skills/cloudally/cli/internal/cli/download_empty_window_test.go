// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"cloudally-pp-cli/internal/cliutil/testenv"
)

// Found by the guard simulation (seed 4): the portal counted a create whose
// job was listed only after the reconcile wait. The count rule (emptyRows)
// read that as an empty job and the row went EMPTY, which nothing revisits:
// the run exited 0 and 'download fetch' skipped an archive that exists.
//
// A count is evidence, not proof: a job that is counted but not listed yet is
// either empty or late. So the row keeps the evidence and stays REQUESTED,
// and becomes EMPTY only when it is still unlisted staleRequestedAge after its
// create was accepted, the same wait a MISSING conclusion needs. A job listed
// in that time confirms the row CREATED.
func TestACountedCreateListedLateIsNeverEmpty(t *testing.T) {
	testenv.Isolate(t)
	s := newDLStub(t)
	prevDelay, prevStale := reconcileDelay, staleRequestedAge
	reconcileDelay, staleRequestedAge = time.Millisecond, time.Hour
	t.Cleanup(func() { reconcileDelay, staleRequestedAge = prevDelay, prevStale })
	s.mu.Lock()
	s.taskEntities = []string{"a@contoso.com"}
	s.countCreates = true // the dashboard counts every create at once
	s.mu.Unlock()         // and the listing does not show them yet

	_, stderr, err := runDL(t, s, downloadSubmitArgs()...)
	if err == nil || ExitCode(err) != 6 {
		t.Fatalf("counted but unlisted creates are not settled: want exit 6, got %v\n%s", err, stderr)
	}
	key := ledgerKeys(t)[0]
	for _, r := range mustLoadBatch(t, key).Rows {
		if r.Status != dlRequested || r.CountedAt == "" {
			t.Fatalf("row %s is %s (counted %q), want REQUESTED with the count recorded", r.SubSource, r.Status, r.CountedAt)
		}
	}
	if !strings.Contains(err.Error(), "counted by the portal but not listed yet") {
		t.Fatalf("the message should say what the rows are waiting for: %v", err)
	}

	// The jobs appear late: a resume confirms them, and creates nothing.
	s.mu.Lock()
	creates := len(s.creates)
	for _, body := range s.createBodies {
		account, _ := body["account"].(string)
		sub, _ := body["source"].(string)
		date, _ := body["date"].(float64)
		label, _ := body["jobLabel"].(string)
		s.jobs = append(s.jobs, dlJob{JobID: fmt.Sprintf("job-late-%d", len(s.jobs)+1), Account: account, SubSource: sub,
			Status: "INPROCESS", Snapshot: int64(date), Label: label, Created: compactNow()})
	}
	s.mu.Unlock()
	if _, stderr, err := runDL(t, s, "download", "submit", "--batch", key, "--resume", "--yes", "--json"); err != nil {
		t.Fatalf("resume after the jobs appeared: %v\n%s", err, stderr)
	}
	for _, r := range mustLoadBatch(t, key).Rows {
		if r.Status != dlCreated || r.JobID == "" {
			t.Fatalf("row %s is %s, want CREATED once its job is listed", r.SubSource, r.Status)
		}
	}
	if n := len(s.createdAccounts()); n != creates {
		t.Fatalf("the resume created %d archive(s) again", n-creates)
	}
}

// A counted create still unlisted when the window has passed is EMPTY.
func TestACountedCreateStillUnlistedAfterTheWindowIsEmpty(t *testing.T) {
	testenv.Isolate(t)
	s := newDLStub(t)
	prevDelay, prevStale := reconcileDelay, staleRequestedAge
	reconcileDelay, staleRequestedAge = time.Millisecond, time.Hour
	t.Cleanup(func() { reconcileDelay, staleRequestedAge = prevDelay, prevStale })
	s.mu.Lock()
	s.taskEntities = []string{"a@contoso.com"}
	s.countCreates = true
	s.mu.Unlock()
	if _, _, err := runDL(t, s, downloadSubmitArgs()...); err == nil || ExitCode(err) != 6 {
		t.Fatalf("first run: want exit 6, got %v", err)
	}
	key := ledgerKeys(t)[0]
	staleRequestedAge = 0 // the window has passed and nothing was listed
	s.mu.Lock()
	creates := len(s.creates)
	s.mu.Unlock()
	if _, stderr, err := runDL(t, s, "download", "submit", "--batch", key, "--resume", "--yes", "--json"); err != nil {
		t.Fatalf("resume after the window: %v\n%s", err, stderr)
	}
	for _, r := range mustLoadBatch(t, key).Rows {
		if r.Status != dlEmpty {
			t.Fatalf("row %s is %s, want EMPTY after the window", r.SubSource, r.Status)
		}
	}
	if n := len(s.createdAccounts()); n != creates {
		t.Fatalf("an EMPTY row was created again (%d)", n-creates)
	}
}

// Found by the guard simulation (seed 20125), the MISSING twin of the case
// above: the final reconcile marked an accepted create MISSING after only the
// reconcile wait, and the next resume, not seeing the late job, created it
// again. An accepted create is MISSING only once it is still unlisted
// staleRequestedAge after its acceptance; until then it stays REQUESTED and
// nothing sends it again.
func TestAnAcceptedCreateListedLateIsNeverResent(t *testing.T) {
	testenv.Isolate(t)
	s := newDLStub(t)
	prevDelay, prevStale := reconcileDelay, staleRequestedAge
	reconcileDelay, staleRequestedAge = time.Millisecond, time.Hour
	t.Cleanup(func() { reconcileDelay, staleRequestedAge = prevDelay, prevStale })
	s.mu.Lock()
	s.taskEntities = []string{"a@contoso.com"} // accepted; listed late; not counted
	s.mu.Unlock()
	_, stderr, err := runDL(t, s, downloadSubmitArgs()...)
	if err == nil || ExitCode(err) != 6 {
		t.Fatalf("want exit 6, got %v\n%s", err, stderr)
	}
	key := ledgerKeys(t)[0]
	for _, r := range mustLoadBatch(t, key).Rows {
		if r.Status != dlRequested {
			t.Fatalf("row %s is %s, want REQUESTED until the window passes", r.SubSource, r.Status)
		}
	}
	creates := len(s.createdAccounts())
	if _, stderr, err := runDL(t, s, "download", "submit", "--batch", key, "--resume", "--yes", "--json"); err == nil || ExitCode(err) != 6 {
		t.Fatalf("resume inside the window: want exit 6, got %v\n%s", err, stderr)
	}
	if n := len(s.createdAccounts()); n != creates {
		t.Fatalf("a resume inside the window created %d archive(s) again", n-creates)
	}
	// Past the window and still unlisted: MISSING, and the next resume sends it.
	staleRequestedAge = 0
	_, _, _ = runDL(t, s, "download", "submit", "--batch", key, "--resume", "--yes", "--json")
	if n := len(s.createdAccounts()); n != 2*creates {
		t.Fatalf("after the window the dropped creates are sent once more: creates %d, want %d", n, 2*creates)
	}
}
