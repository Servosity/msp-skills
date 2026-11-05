// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"strings"
	"testing"
	"time"

	"cloudally-pp-cli/internal/cliutil/testenv"
)

// Found by the guard simulation (seed 1014): a batch whose archive another
// batch holds lists that row SKIPPED and counted it settled, so the run exited
// 0 while the holder's create had been accepted but never appeared (MISSING
// in the holder's ledger): the archive did not exist. A held row is settled
// only when its holder's row is: a confirmed job, or EMPTY. Otherwise the run
// is not settled (exit 6) and names the holder batch to settle first.
func TestAHeldRowIsSettledOnlyWhenItsHolderIs(t *testing.T) {
	testenv.Isolate(t)
	s := newDLStub(t)
	prev := reconcileDelay
	reconcileDelay = time.Millisecond
	t.Cleanup(func() { reconcileDelay = prev })
	s.mu.Lock()
	s.taskEntities = []string{"a@contoso.com", "b@contoso.com"}
	s.listCreated = false // the pilot's creates are accepted but never appear
	s.mu.Unlock()
	_, _, err := runDL(t, s, downloadSubmitArgs("--entities", "a@contoso.com")...)
	if err == nil || ExitCode(err) != 6 {
		t.Fatalf("pilot: want exit 6 with its rows MISSING, got %v", err)
	}
	pilot := ledgerKeys(t)[0]

	s.mu.Lock()
	s.listCreated = true // the whole task's own creates appear
	s.mu.Unlock()
	_, stderr, err := runDL(t, s, downloadSubmitArgs()...)
	if err == nil || ExitCode(err) != 6 {
		t.Fatalf("whole task: a@'s archives are held by a batch that has not created them, want exit 6, got %v\n%s", err, stderr)
	}
	if !strings.Contains(err.Error(), "--batch "+pilot+" --resume --yes") {
		t.Fatalf("the message should name the holder's resume: %v", err)
	}
	if strings.Contains(err.Error()+stderr, "--new-batch") {
		t.Fatalf("a held row must not steer to --new-batch: %v\n%s", err, stderr)
	}

	// Once the holder settles them (its resume finds the jobs), the whole
	// task's resume settles too, and nothing is created twice.
	s.mu.Lock()
	for _, body := range s.createBodies {
		if acct, _ := body["account"].(string); acct == "a@contoso.com" {
			sub, _ := body["source"].(string)
			date, _ := body["date"].(float64)
			label, _ := body["jobLabel"].(string)
			s.jobs = append(s.jobs, dlJob{JobID: "job-pilot-" + sub, Account: acct, SubSource: sub, Status: "INPROCESS",
				Snapshot: int64(date), Label: label, Created: compactNow()})
		}
	}
	creates := len(s.creates)
	s.mu.Unlock()
	if _, stderr, err := runDL(t, s, "download", "submit", "--batch", pilot, "--resume", "--yes", "--json"); err != nil {
		t.Fatalf("holder resume: %v\n%s", err, stderr)
	}
	var whole string
	for _, k := range ledgerKeys(t) {
		if k != pilot {
			whole = k
		}
	}
	if _, stderr, err := runDL(t, s, "download", "submit", "--batch", whole, "--resume", "--yes", "--json"); err != nil {
		t.Fatalf("whole task resume after the holder settled: %v\n%s", err, stderr)
	}
	if n := len(s.createdAccounts()); n != creates {
		t.Fatalf("settling created %d archive(s) again", n-creates)
	}
}

// settleHeldRuns resumes each batch a run left unsettled only because a row
// was held by another batch that had not confirmed it yet: once that batch
// has finished, the resume settles (exit 0) and creates nothing held.
func settleHeldRuns(t *testing.T, s *dlStub, res []childResult) {
	t.Helper()
	for _, r := range res {
		if r.code == 0 {
			continue
		}
		// A row the other run holds, or a reconcile the other run's live
		// create blocked: either way the run sends nothing twice, leaves
		// the rows REQUESTED and says to run again (checked below).
		if r.code != 6 || !(strings.Contains(r.stderr, "which has not created it yet") || strings.Contains(r.stderr, "rows stay REQUESTED and are unverified")) {
			t.Fatalf("a run may end unsettled only for a held row, got %+v", r)
		}
	}
	for _, k := range ledgerKeys(t) {
		if _, stderr, err := runDL(t, s, "download", "submit", "--batch", k, "--resume", "--yes", "--json"); err != nil {
			t.Fatalf("batch %s should settle once every holder has: %v\n%s", k, err, stderr)
		}
	}
}
