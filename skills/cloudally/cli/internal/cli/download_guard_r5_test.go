// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"cloudally-pp-cli/internal/cliutil/testenv"
	"cloudally-pp-cli/internal/portal"
)

// runDLContext is runDL with a context the test controls.
func runDLContext(t *testing.T, ctx context.Context, s *dlStub, args ...string) (string, string, error) {
	t.Helper()
	t.Setenv("CLOUDALLY_BASE_URL", s.api.URL)
	cmd := RootCmd()
	cmd.SetArgs(append(args, "--portal-url", s.portal.URL))
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	err := cmd.ExecuteContext(ctx)
	return out.String(), errOut.String(), err
}

// guardClaimRow reads the live (or latest) claim for one sub-source.
func guardClaimRow(t *testing.T, g *downloadGuard, sub string) (state, batch, jobID string, deleted, hold int) {
	t.Helper()
	err := g.db.QueryRow(`SELECT state, batch_key, job_id, include_deleted, include_legal_hold FROM download_claims
		WHERE sub_source = ? AND superseded = 0 ORDER BY id DESC LIMIT 1`, sub).Scan(&state, &batch, &jobID, &deleted, &hold)
	if err != nil {
		t.Fatalf("reading the guard: %v", err)
	}
	return
}

// insertClaim writes a claim straight into the guard, as another run or an
// older build would have left it.
func insertClaim(t *testing.T, g *downloadGuard, sub, state, batch, owner, label string) {
	t.Helper()
	now := isoUTC(time.Now())
	if _, err := g.db.Exec(`INSERT INTO download_claims
		(account_id, task_id, entity, sub_source, snapshot, format, include_deleted, include_legal_hold, state, batch_key, label, owner_run, created_at, updated_at)
		VALUES ('acct-1', 'task-1', 'a@contoso.com', ?, 20221231042000, 'zip', 0, 0, ?, ?, ?, ?, ?, ?)`,
		sub, state, batch, label, owner, now, now); err != nil {
		t.Fatal(err)
	}
}

// P1-a: a deferred create whose poll is refused may still have been
// accepted; it is UNKNOWN and a resume never sends it again on its own.
func TestDownloadDeferredCreateThenAuthFailureIsUnknown(t *testing.T) {
	err := fmt.Errorf("%w: %w", portal.ErrOutcomeUnknown, fmt.Errorf("portal: startRestoreOrDownload: %w (status 401)", portal.ErrNoSession))
	if got := classifyDownloadCreateFailure(err, true); got != dlUnknown {
		t.Fatalf("a refused poll after a deferred create is %s, want UNKNOWN", got)
	}
	testenv.Isolate(t)
	s := newDLStub(t)
	quickReconcile(t, s)
	s.mu.Lock()
	s.taskEntities, s.deferCreate, s.pollCode, s.listCreated = []string{"a@contoso.com"}, true, 401, false
	s.mu.Unlock()
	_, _, runErr := runDL(t, s, downloadSubmitArgs("--sub-sources", "O365MAIL", "--no-reconcile")...)
	if runErr == nil || ExitCode(runErr) != 6 {
		t.Fatalf("want exit 6 for an unknown outcome, got %v", runErr)
	}
	key := ledgerKeys(t)[0]
	if r := mustLoadBatch(t, key).Rows[0]; r.Status != dlUnknown {
		t.Fatalf("row = %+v, want UNKNOWN", r)
	}
	s.mu.Lock()
	s.deferCreate = false
	s.mu.Unlock()
	_, _, _ = runDL(t, s, "download", "submit", "--batch", key, "--resume", "--yes")
	if n := len(s.createdAccounts()); n != 1 {
		t.Fatalf("creates = %d, want 1: the deferred create may exist", n)
	}
}

// P1-b: evidence from every ledger is merged, the confirmed job winning
// whichever ledger is read first.
func TestDownloadSeedingMergesEvidenceAcrossLedgers(t *testing.T) {
	for _, order := range []string{"together", "missing first", "created first"} {
		t.Run(order, func(t *testing.T) {
			testenv.Isolate(t)
			s := newDLStub(t)
			quickReconcile(t, s)
			missing := downloadRow{Entity: "a@contoso.com", SubSource: "O365MAIL", SnapshotRaw: 20221231042000, Status: dlMissing, AcceptedAt: "2026-10-15T20:15:00Z"}
			created := downloadRow{Entity: "a@contoso.com", SubSource: "O365MAIL", SnapshotRaw: 20221231042000, Status: dlCreated, JobID: "job-b"}
			const keyA, keyB = "20261001-000000-a", "20261002-000000-b"
			ctx := context.Background()
			switch order {
			case "together":
				writeLegacyLedger(t, keyA, []downloadRow{missing})
				writeLegacyLedger(t, keyB, []downloadRow{created})
			case "missing first":
				writeLegacyLedger(t, keyA, []downloadRow{missing})
				if err := testGuard(t).seedFromLedgers(ctx); err != nil {
					t.Fatal(err)
				}
				writeLegacyLedger(t, keyB, []downloadRow{created})
			case "created first":
				writeLegacyLedger(t, keyB, []downloadRow{created})
				if err := testGuard(t).seedFromLedgers(ctx); err != nil {
					t.Fatal(err)
				}
				writeLegacyLedger(t, keyA, []downloadRow{missing})
			}
			g := testGuard(t)
			if err := g.seedFromLedgers(ctx); err != nil {
				t.Fatal(err)
			}
			if state, batch, job, _, _ := guardClaimRow(t, g, "O365MAIL"); state != dgCreated || batch != keyB || job != "job-b" {
				t.Fatalf("merged claim = %s %s %s, want CREATED by %s pinned to job-b", state, batch, job, keyB)
			}
			_, _, _ = runDL(t, s, "download", "submit", "--batch", keyA, "--resume", "--include-deleted=false", "--include-legal-hold=false", "--yes")
			if n := len(s.createdAccounts()); n != 0 {
				t.Fatalf("resuming A created %d export(s) of an archive batch B confirmed", n)
			}
		})
	}
}

// runAPI runs the CLI against a documented-API stub without re-isolating.
func runAPI(t *testing.T, apiURL string, args ...string) (string, string, error) {
	t.Helper()
	t.Setenv("CLOUDALLY_BASE_URL", apiURL)
	cmd := RootCmd()
	cmd.SetArgs(args)
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	err := cmd.Execute()
	return out.String(), errOut.String(), err
}

// P1-c: download start claims its archive in the guard, so the same start
// twice creates one job.
func TestDownloadStartTwiceCreatesOnce(t *testing.T) {
	testenv.Isolate(t)
	f := &fakeCloudAlly{t: t, taskSource: "GMAIL"}
	api := f.apiServerWithHandoff()
	portalSrv := f.portalServer()
	args := []string{"download", "start", "--account", "acct-1", "--task", "task-1",
		"--entity", "jane.doe@contoso.com", "--date", "2025-12-31", "--portal-url", portalSrv.URL, "--yes", "--json"}
	out, stderr, err := runAPI(t, api.URL, args...)
	if err != nil {
		t.Fatalf("first start: %v\n%s", err, stderr)
	}
	_, stderr2, err2 := runAPI(t, api.URL, args...)
	if n := len(f.posts()); n != 1 {
		t.Fatalf("the create was sent %d times, want once", n)
	}
	if err2 == nil || ExitCode(err2) != 2 {
		t.Fatalf("the same start again must be refused, got %v\n%s", err2, stderr2)
	}
	var first downloadStartOutput
	if jerr := json.Unmarshal([]byte(out), &first); jerr != nil || first.BatchKey == "" {
		t.Fatalf("the start output should name its batch: %v %s", jerr, out)
	}
}

// P1-d: EMPTY is inferred only when the job count can only be this batch's.
func TestDownloadEmptyNeedsTheCountAlone(t *testing.T) {
	cases := []struct {
		name      string
		extra     int
		otherWork bool
		want      string
		code      int
	}{
		{"alone", 0, false, dlEmpty, 0},
		{"another batch ran", 0, true, dlMissing, 6},
		{"the count grew by more", 2, false, dlMissing, 6},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			testenv.Isolate(t)
			s := newDLStub(t)
			prev := reconcileDelay
			reconcileDelay = time.Millisecond
			t.Cleanup(func() { reconcileDelay = prev })
			g := testGuard(t)
			s.mu.Lock()
			s.taskEntities = []string{"a@contoso.com"}
			s.countCreates = true
			s.onDashboard = func(call int) {
				if call != 2 {
					return
				}
				// What happened on the account while this batch waited.
				s.dashboardExtra = tc.extra
				if tc.otherWork {
					insertClaim(t, g, "O365NOTES", dgCreated, "20221231-000000-other", "", "")
				}
			}
			s.mu.Unlock()
			// The creates are answered but no job appears: dropped, or empty.
			_, stderr, err := runDL(t, s, downloadSubmitArgs()...)
			got := 0
			if err != nil {
				got = ExitCode(err)
			}
			if got != tc.code {
				t.Fatalf("exit %d, want %d (%v)\n%s", got, tc.code, err, stderr)
			}
			for _, r := range mustLoadBatch(t, ledgerKeys(t)[0]).Rows {
				if r.Status != tc.want {
					t.Fatalf("row %s is %s, want %s", r.SubSource, r.Status, tc.want)
				}
			}
		})
	}
}

// P1-e: a pilot, then the whole task: the whole run creates only what the
// pilot did not, and settles. An unsettled pilot still blocks it.
func TestDownloadPilotThenWholeTaskCreatesOnlyTheRest(t *testing.T) {
	testenv.Isolate(t)
	s := newDLStub(t)
	quickReconcile(t, s)
	s.taskEntities = []string{"a@contoso.com", "b@contoso.com"}
	if _, stderr, err := runDL(t, s, downloadSubmitArgs("--entities", "a@contoso.com")...); err != nil {
		t.Fatalf("pilot: %v\n%s", err, stderr)
	}
	pilot := ledgerKeys(t)[0]
	if _, stderr, err := runDL(t, s, downloadSubmitArgs()...); err != nil {
		t.Fatalf("the whole task after a pilot should settle: %v\n%s", err, stderr)
	}
	if got := sortedCopy(s.createdAccounts()); len(got) != 4 || got[2] != "b@contoso.com" || got[3] != "b@contoso.com" {
		t.Fatalf("creates = %v, want a@ twice (pilot) and b@ twice (whole)", got)
	}
	var whole string
	for _, k := range ledgerKeys(t) {
		if k != pilot {
			whole = k
		}
	}
	for _, r := range mustLoadBatch(t, whole).Rows {
		if r.Entity == "a@contoso.com" && (r.Status != dlSkipped || r.HeldBy != pilot) {
			t.Fatalf("a@ in the whole batch should be SKIPPED, held by the pilot: %+v", r)
		}
	}
	out, _, _ := runDL(t, s, fetchArgs(t.TempDir(), "--batch", whole)...)
	if !strings.Contains(out, fetchSkippedHeld) || !strings.Contains(out, "--batch "+pilot) {
		t.Fatalf("fetch --batch should list a@ as held by the pilot:\n%s", out)
	}

	// A pilot whose create outcome is unknown blocks the whole task.
	testenv.Isolate(t)
	s2 := newDLStub(t)
	quickReconcile(t, s2)
	s2.taskEntities = []string{"c@contoso.com", "d@contoso.com"}
	s2.createCode["c@contoso.com"] = 500
	_, _, _ = runDL(t, s2, downloadSubmitArgs("--entities", "c@contoso.com")...)
	delete(s2.createCode, "c@contoso.com")
	before := len(s2.createdAccounts())
	if _, _, err := runDL(t, s2, downloadSubmitArgs()...); err == nil || ExitCode(err) != 2 {
		t.Fatalf("an unsettled pilot must block the whole task, got %v", err)
	}
	if n := len(s2.createdAccounts()); n != before {
		t.Fatalf("a refused run created %d", n-before)
	}
}

// P1-f(1): opening the guard turns abandoned IN_FLIGHT claims into UNKNOWN.
func TestOpeningTheGuardRecoversAbandonedClaims(t *testing.T) {
	testenv.Isolate(t)
	g := testGuard(t)
	insertClaim(t, g, "O365MAIL", dgInFlight, "20221231-000000-dead", "999-deadbeefdeadbeef", rowLabelPrefix+"dead")
	g.Close()
	g2 := testGuard(t)
	if state, _, _, _, _ := guardClaimRow(t, g2, "O365MAIL"); state != dgUnknown {
		t.Fatalf("an abandoned IN_FLIGHT claim is %s after opening the guard, want UNKNOWN", state)
	}
}

// P1-f(2): pin and hold accept a claim whose owner is gone, and settle
// reports a claim it no longer holds.
func TestPinHoldAndSettleOnRecoveredClaims(t *testing.T) {
	testenv.Isolate(t)
	ctx := context.Background()
	g := testGuard(t)
	id := downloadIdentity{Account: "acct-1", Task: "task-1", Entity: "a@contoso.com", SubSource: "O365MAIL", Snapshot: 20221231042000, Format: "zip"}
	insertClaim(t, g, "O365MAIL", dgInFlight, "20221231-000000-k", "999-deadbeefdeadbeef", "")
	if err := g.pin(ctx, id, "20221231-000000-k", "job-1"); err != nil {
		t.Fatalf("pin on an abandoned claim: %v", err)
	}
	if state, _, job, _, _ := guardClaimRow(t, g, "O365MAIL"); state != dgCreated || job != "job-1" {
		t.Fatalf("pinned claim = %s %s", state, job)
	}
	idCal := id
	idCal.SubSource = "O365CALENDAR"
	insertClaim(t, g, "O365CALENDAR", dgInFlight, "20221231-000000-k", "999-deadbeefdeadbeef", "")
	if err := g.hold(ctx, idCal, "20221231-000000-k"); err != nil {
		t.Fatalf("hold on an abandoned claim: %v", err)
	}
	if state, _, _, _, _ := guardClaimRow(t, g, "O365CALENDAR"); state != dgUnknown {
		t.Fatalf("held claim = %s", state)
	}
	if err := g.settle(ctx, 424242, dlRequested, true); err == nil {
		t.Fatal("settling a claim the guard does not hold must be an error")
	}
}

// P1-f(3): a label the guard recorded for this row's own earlier, failed
// create is not this row's job, so it cannot make the row ambiguous.
func TestReconcileIgnoresTheRowsOwnEarlierFailedJob(t *testing.T) {
	testenv.Isolate(t)
	ctx := context.Background()
	g := testGuard(t)
	insertClaim(t, g, "O365MAIL", dgJobFailed, "20221231-000000-k", "", rowLabelPrefix+"first")
	b := &downloadBatch{BatchKey: "20221231-000000-k", AccountID: "acct-1", TaskID: "task-1", Format: "zip",
		IncludeDeleted: new(bool), IncludeLegalHold: new(bool),
		Rows: []downloadRow{{Entity: "a@contoso.com", SubSource: "O365MAIL", SnapshotRaw: 20221231042000, Status: dlRequested,
			Label: rowLabelPrefix + "second", RequestedAt: isoUTC(time.Now())}}}
	old := portal.DownloadJob{JobID: "job-first", Account: "a@contoso.com", SubSource: "O365MAIL", SnapshotDate: 20221231042000,
		Status: "FAILED", JobLabel: rowLabelPrefix + "first", InitDate: compactAt(time.Now().Add(-time.Hour))}
	res, err := reconcileRows(ctx, g, b, []portal.DownloadJob{old}, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.ambiguous != 0 || b.Rows[0].Ambiguous {
		t.Fatalf("the row's own earlier failed job made it ambiguous: %+v", b.Rows[0])
	}
}

// P1-f, whole: a run killed after its create was accepted; the resume
// confirms the job; the job fails; fetch and a resume create it exactly
// once more, and the batch settles.
func TestDownloadKilledAfterAcceptRecoversAndRecreatesOnce(t *testing.T) {
	if testing.Short() {
		t.Skip("starts a child process")
	}
	testenv.Isolate(t)
	s := newDLStub(t)
	quickReconcile(t, s)
	s.mu.Lock()
	s.taskEntities, s.createDelay = []string{"a@contoso.com"}, 1500*time.Millisecond
	s.mu.Unlock()
	raw, _ := json.Marshal(append(downloadSubmitArgs("--sub-sources", "O365MAIL"), "--portal-url", s.portal.URL))
	child := exec.Command(os.Args[0], "-test.run=^TestHelperDownloadSubmitProcess$", "-test.count=1")
	child.Env = append(os.Environ(), helperFlag+"=1", helperArgs+"="+string(raw), helperHome+"="+os.Getenv("HOME"), "CLOUDALLY_BASE_URL="+s.api.URL)
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(20 * time.Second)
	for len(s.createdAccounts()) == 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	_ = child.Process.Kill() // SIGKILL while the accepted create's answer is in flight
	_ = child.Wait()
	for time.Now().Before(deadline) {
		s.mu.Lock()
		listed := len(s.jobs)
		s.mu.Unlock()
		if listed > 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	s.mu.Lock()
	s.createDelay = 0
	s.mu.Unlock()
	key := ledgerKeys(t)[0]

	if _, stderr, err := runDL(t, s, "download", "submit", "--batch", key, "--resume", "--yes"); err != nil {
		t.Fatalf("the resume should confirm the job: %v\n%s", err, stderr)
	}
	if r := mustLoadBatch(t, key).Rows[0]; r.Status != dlCreated || r.JobID == "" {
		t.Fatalf("row = %+v, want CREATED", r)
	}
	s.mu.Lock()
	s.jobs[0].Status = "FAILED"
	s.mu.Unlock()
	if _, _, err := runDL(t, s, fetchArgs(t.TempDir(), "--batch", key)...); err == nil || ExitCode(err) != 6 {
		t.Fatalf("fetch of the failed job: want exit 6, got %v", err)
	}
	if _, stderr, err := runDL(t, s, "download", "submit", "--batch", key, "--resume", "--yes"); err != nil {
		t.Fatalf("the resume that creates it once more: %v\n%s", err, stderr)
	}
	if n := len(s.createdAccounts()); n != 2 {
		t.Fatalf("creates = %d, want 2", n)
	}
	if _, stderr, err := runDL(t, s, "download", "submit", "--batch", key, "--resume", "--yes"); err != nil {
		t.Fatalf("the batch should settle: %v\n%s", err, stderr)
	}
	if n := len(s.createdAccounts()); n != 2 {
		t.Fatalf("creates = %d after settling, want 2", n)
	}
}

// P2-a, round 6 P1-2: every FAILED row of an older ledger is held as
// possibly created. That build wrote FAILED for every create error, and no
// error text proves a request never reached the portal (a refused
// connection to a proxy, or a cached DNS failure on one path, says nothing
// about a retry that did get through), so no text heuristic releases one.
func TestLegacyFailedRowsAreAlwaysHeld(t *testing.T) {
	testenv.Isolate(t)
	s := newDLStub(t)
	quickReconcile(t, s)
	s.taskEntities = []string{"a@contoso.com"}
	writeLegacyLedger(t, "20261015-201500-old", []downloadRow{
		{Entity: "a@contoso.com", SubSource: "O365MAIL", SnapshotRaw: 20221231042000, Status: dlFailed, Detail: "portal: startRestoreOrDownload: status 502"},
		{Entity: "a@contoso.com", SubSource: "O365CALENDAR", SnapshotRaw: 20221231041000, Status: dlFailed, Detail: "portal: POST startRestoreOrDownload: dial tcp 10.0.0.1:443: connect: connection refused"},
	})
	_, stderr, _ := runDL(t, s, "download", "submit", "--batch", "20261015-201500-old", "--resume", "--yes")
	if strings.Contains(stderr, "--sub-sources O365CALENDAR --yes") || strings.Contains(stderr, "--sub-sources O365MAIL --yes") {
		t.Fatalf("no legacy FAILED row may be offered as never sent:\n%s", stderr)
	}
	for _, sub := range []string{"O365MAIL", "O365CALENDAR"} {
		if _, _, err := runDL(t, s, downloadSubmitArgs("--sub-sources", sub)...); err == nil || ExitCode(err) != 2 {
			t.Fatalf("legacy FAILED %s may exist and must block a new batch, got %v", sub, err)
		}
		if state, _, _, _, _ := guardClaimRow(t, testGuard(t), sub); state != dgUnknown {
			t.Fatalf("legacy FAILED %s seeded as %q, want UNKNOWN", sub, state)
		}
	}
	if got := s.createdAccounts(); len(got) != 0 {
		t.Fatalf("creates = %v, want none", got)
	}
}

// P2-b(i): a legacy resume's stated flags narrow its seeded claims, so a
// legal-hold export of the same snapshot is not blocked by them.
func TestLegacyResumeNarrowsSeededFlags(t *testing.T) {
	testenv.Isolate(t)
	s := newDLStub(t)
	quickReconcile(t, s)
	s.taskEntities = []string{"a@contoso.com"}
	const key = "20261015-201500-old"
	writeLegacyLedger(t, key, []downloadRow{{Entity: "a@contoso.com", SubSource: "O365MAIL", SnapshotRaw: 20221231042000, Status: dlCreated, JobID: "job-a"}})
	if _, stderr, err := runDL(t, s, "download", "submit", "--batch", key, "--resume", "--include-deleted=false", "--include-legal-hold=false", "--yes"); err != nil {
		t.Fatalf("legacy resume: %v\n%s", err, stderr)
	}
	if _, _, _, deleted, hold := guardClaimRow(t, testGuard(t), "O365MAIL"); deleted != 0 || hold != 0 {
		t.Fatalf("seeded flags = %d/%d, want 0/0 once stated", deleted, hold)
	}
	if _, stderr, err := runDL(t, s, downloadSubmitArgs("--sub-sources", "O365MAIL", "--include-legal-hold")...); err != nil {
		t.Fatalf("legal-hold export: %v\n%s", err, stderr)
	}
	if n := len(s.createdAccounts()); n != 1 {
		t.Fatalf("creates = %d, want the legal-hold export once", n)
	}
}

// P2-b(ii): --new-batch supersedes only a claim with exactly its flags; a
// claim whose flags were never recorded keeps blocking the others.
func TestNewBatchSupersedesOnlyExactFlagMatches(t *testing.T) {
	testenv.Isolate(t)
	s := newDLStub(t)
	quickReconcile(t, s)
	s.taskEntities = []string{"a@contoso.com"}
	writeLegacyLedger(t, "20261015-201500-old", []downloadRow{{Entity: "a@contoso.com", SubSource: "O365MAIL", SnapshotRaw: 20221231042000, Status: dlCreated, JobID: "job-a"}})
	if _, stderr, err := runDL(t, s, downloadSubmitArgs("--sub-sources", "O365MAIL", "--new-batch")...); err != nil {
		t.Fatalf("--new-batch: %v\n%s", err, stderr)
	}
	if _, stderr, err := runDL(t, s, downloadSubmitArgs("--sub-sources", "O365MAIL", "--include-deleted")...); err != nil {
		t.Fatalf("include-deleted run: %v\n%s", err, stderr)
	}
	if n := len(s.createdAccounts()); n != 1 {
		t.Fatalf("creates = %d, want 1: the older export (flags unknown) still holds the include-deleted archive", n)
	}
}

// P2-c: a reconcile interrupted by Ctrl-C leaves an unsettled batch (exit
// 6) and says how to continue it.
func TestDownloadInterruptedReconcileExitsSix(t *testing.T) {
	testenv.Isolate(t)
	s := newDLStub(t)
	prev := reconcileDelay
	reconcileDelay = 5 * time.Second
	t.Cleanup(func() { reconcileDelay = prev })
	s.taskEntities = []string{"a@contoso.com"}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.onCreate = func(map[string]any) { time.AfterFunc(200*time.Millisecond, cancel) }
	_, _, err := runDLContext(t, ctx, s, downloadSubmitArgs("--sub-sources", "O365MAIL")...)
	if err == nil || ExitCode(err) != 6 {
		t.Fatalf("an interrupted reconcile must exit 6, got %v", err)
	}
	if key := ledgerKeys(t)[0]; !strings.Contains(err.Error(), "--batch "+key+" --resume --yes") {
		t.Fatalf("the error should name the resume command: %v", err)
	}
}

// P2-d: a snapshot-wide fetch does not report a failed job its batch has
// already created again.
func TestSnapshotFetchIgnoresAReplacedFailedJob(t *testing.T) {
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
	_, _, _ = runDL(t, s, fetchArgs(t.TempDir(), "--batch", key)...)
	if _, stderr, err := runDL(t, s, "download", "submit", "--batch", key, "--resume", "--yes"); err != nil {
		t.Fatalf("re-create: %v\n%s", err, stderr)
	}
	s.mu.Lock()
	j := &s.jobs[len(s.jobs)-1]
	j.Status = portal.StatusReadyForDownload
	j.Files = []portal.DownloadFile{{ExportDataID: "d-new", FileName: "a.zip", Size: "6 Bytes"}}
	s.bodies["d-new"] = []byte("PK\x03\x04ok")
	s.mu.Unlock()
	out, stderr, err := runDL(t, s, fetchArgs(t.TempDir(), "--on-date", "2022-12-31", "--entities", "a@contoso.com")...)
	if err != nil {
		t.Fatalf("the snapshot-wide fetch should finish once the job was re-created: %v\n%s\n%s", err, out, stderr)
	}
	if strings.Contains(out, "once more") {
		t.Fatalf("stale advice for a replaced job:\n%s", out)
	}
}

// P2-e: a confirmed archive the portal no longer lists is reported as
// expired, with a person's way to create it again.
func TestFetchSaysWhenAConfirmedArchiveExpired(t *testing.T) {
	testenv.Isolate(t)
	s := newDLStub(t)
	b := newSubmitBatch(t, "20221231-000000-expired", "a@contoso.com")
	b.Rows[0].Status, b.Rows[0].JobID = dlCreated, "job-gone"
	if err := saveBatch(b); err != nil {
		t.Fatal(err)
	}
	out, _, err := runDL(t, s, fetchArgs(t.TempDir(), "--batch", b.BatchKey)...)
	if err == nil || ExitCode(err) != 6 {
		t.Fatalf("want exit 6, got %v", err)
	}
	for _, want := range []string{"expired", "a person can decide", "--entities a@contoso.com --sub-sources O365MAIL --new-batch --yes"} {
		if !strings.Contains(out, want) {
			t.Fatalf("fetch should say %q:\n%s", want, out)
		}
	}
}

// fetch --utc-offset reads --on-date as submit does: the account's day.
func TestFetchUTCOffsetMatchesSubmitsDay(t *testing.T) {
	testenv.Isolate(t)
	s := newDLStub(t)
	s.addFile("j-mail", jane, "O365MAIL", "d1", "jane.zip", []byte("PK\x03\x04mail"))
	s.mu.Lock()
	s.jobs[0].Snapshot = 20230101031000 // 22:10 on 31 December at UTC-5
	s.mu.Unlock()
	if _, _, err := runDL(t, s, fetchArgs(t.TempDir(), "--on-date", "2022-12-31")...); err == nil || ExitCode(err) != 3 {
		t.Fatalf("by the UTC day it is 1 January: want exit 3, got %v", err)
	}
	if _, stderr, err := runDL(t, s, fetchArgs(t.TempDir(), "--on-date", "2022-12-31", "--utc-offset", "-5")...); err != nil {
		t.Fatalf("by the account's day it is 31 December: %v\n%s", err, stderr)
	}
}
