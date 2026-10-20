package cli

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"cloudally-pp-cli/internal/cliutil/testenv"
	"cloudally-pp-cli/internal/portal"
)

// Round 6 P1-1: a resend of a create that may already exist can never
// release it. The first create was accepted but its answer was lost (a
// 502), so the row is UNKNOWN. A --retry-unknown resend refused with a 403
// answers only that resend: the row and its claim stay UNKNOWN, and a
// plain --resume sends nothing. The old code recorded the refusal as
// FAILED (guard REJECTED), and the plain --resume created it again.
func TestRefusedRetryOfAnUnknownCreateKeepsItUnknown(t *testing.T) {
	testenv.Isolate(t)
	s := newDLStub(t)
	quickReconcile(t, s)
	s.taskEntities = []string{"a@contoso.com"}
	s.createCode["a@contoso.com"] = http.StatusBadGateway
	_, _, _ = runDL(t, s, downloadSubmitArgs("--sub-sources", "O365MAIL")...)
	key := ledgerKeys(t)[0]
	if b, _ := loadBatch(key); b.Rows[0].Status != dlUnknown {
		t.Fatalf("a 502 after the request left must be UNKNOWN, got %+v", b.Rows[0])
	}

	s.mu.Lock()
	s.createCode["a@contoso.com"] = http.StatusForbidden
	s.mu.Unlock()
	_, _, _ = runDL(t, s, "download", "submit", "--batch", key, "--resume", "--retry-unknown", "--yes")
	if n := len(s.createdAccounts()); n != 2 {
		t.Fatalf("creates = %d, want 2 (the original and the refused resend)", n)
	}

	s.mu.Lock()
	s.createCode["a@contoso.com"] = 0
	s.mu.Unlock()
	_, _, _ = runDL(t, s, "download", "submit", "--batch", key, "--resume", "--yes")
	_, _, _ = runDL(t, s, downloadSubmitArgs("--sub-sources", "O365MAIL", "--entities", "a@contoso.com")...)
	if n := len(s.createdAccounts()); n != 2 {
		t.Fatalf("creates = %d after a plain resume and a new batch, want still 2", n)
	}
	b, _ := loadBatch(key)
	if b.Rows[0].Status != dlUnknown {
		t.Fatalf("a refused resend must leave the row UNKNOWN, got %+v", b.Rows[0])
	}
	if state, _, _, _, _ := guardClaimRow(t, testGuard(t), "O365MAIL"); state != dgUnknown {
		t.Fatalf("a refused resend must leave the claim UNKNOWN, got %s", state)
	}
}

// The same class for a MISSING row: its first create was accepted (the
// claim is CREATED, unconfirmed). A refused resend must not free it.
func TestRefusedResendOfAMissingRowDoesNotFreeIt(t *testing.T) {
	testenv.Isolate(t)
	s := newDLStub(t)
	quickReconcile(t, s)
	s.mu.Lock()
	s.listCreated = false // accepted, never listed: MISSING
	s.taskEntities = []string{"a@contoso.com"}
	s.mu.Unlock()
	_, _, _ = runDL(t, s, downloadSubmitArgs("--sub-sources", "O365MAIL")...)
	key := ledgerKeys(t)[0]
	if b, _ := loadBatch(key); b.Rows[0].Status != dlMissing {
		t.Fatalf("want MISSING, got %+v", b.Rows[0])
	}
	s.mu.Lock()
	s.createCode["a@contoso.com"] = http.StatusForbidden
	s.mu.Unlock()
	_, _, _ = runDL(t, s, "download", "submit", "--batch", key, "--resume", "--yes")
	s.mu.Lock()
	s.createCode["a@contoso.com"] = 0
	s.mu.Unlock()
	_, _, _ = runDL(t, s, "download", "submit", "--batch", key, "--resume", "--yes")
	_, _, _ = runDL(t, s, downloadSubmitArgs("--sub-sources", "O365MAIL", "--entities", "a@contoso.com")...)
	if n := len(s.createdAccounts()); n != 2 {
		t.Fatalf("creates = %d, want 2 (the original and the refused resend)", n)
	}
	if state, _, _, _, _ := guardClaimRow(t, testGuard(t), "O365MAIL"); state == dgRejected {
		t.Fatal("a refused resend of an accepted create freed its claim")
	}
}

// P1-1, the rollback path: the ledger cannot be saved right after an
// UNKNOWN row's claim is reused, so the resend is never sent. The rollback
// must leave the claim UNKNOWN; the old code released it (REJECTED), and a
// new batch then created the archive a second time.
func TestRolledBackRetryOfAnUnknownCreateKeepsItUnknown(t *testing.T) {
	testenv.Isolate(t)
	s := newDLStub(t)
	quickReconcile(t, s)
	s.taskEntities = []string{"a@contoso.com"}
	s.createCode["a@contoso.com"] = http.StatusBadGateway
	_, _, _ = runDL(t, s, downloadSubmitArgs("--sub-sources", "O365MAIL")...)
	key := ledgerKeys(t)[0]
	b, err := loadBatch(key)
	if err != nil || b.Rows[0].Status != dlUnknown {
		t.Fatalf("want an UNKNOWN row, got %+v %v", b, err)
	}

	// A directory where the ledger file was makes every save fail.
	path, _ := batchPath(key)
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(path, "blocker"), 0o700); err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	s.createCode["a@contoso.com"] = 0
	s.mu.Unlock()
	g := testGuard(t)
	if err := runDownloadBatch(context.Background(), quietCmd(), openStubSession(t, s), g, b, pendingDownloadRows(b, true, true), 1, true, false); err == nil {
		t.Fatal("a resend whose ledger cannot be saved must report it was not sent")
	}
	if n := len(s.createdAccounts()); n != 1 {
		t.Fatalf("creates = %d, want 1: the rolled-back resend must not be sent", n)
	}
	if b.Rows[0].Status != dlUnknown {
		t.Fatalf("the rollback must leave the row UNKNOWN, got %+v", b.Rows[0])
	}
	if state, _, _, _, _ := guardClaimRow(t, g, "O365MAIL"); state != dgUnknown {
		t.Fatalf("the rollback must leave the claim UNKNOWN, got %s", state)
	}

	if err := os.RemoveAll(path); err != nil {
		t.Fatal(err)
	}
	if err := saveBatch(b); err != nil {
		t.Fatal(err)
	}
	_, _, _ = runDL(t, s, downloadSubmitArgs("--sub-sources", "O365MAIL", "--entities", "a@contoso.com")...)
	_, _, _ = runDL(t, s, "download", "submit", "--batch", key, "--resume", "--yes")
	if n := len(s.createdAccounts()); n != 1 {
		t.Fatalf("creates = %d after a new batch and a plain resume, want still 1", n)
	}
	if state, _, _, _, _ := guardClaimRow(t, g, "O365MAIL"); state != dgUnknown {
		t.Fatalf("the rollback must leave the claim UNKNOWN, got %s", state)
	}
}

// The rollback where the old code made a duplicate: a legacy FAILED row is
// held in the guard as UNKNOWN (it may have been created). Its
// --retry-unknown resend is rolled back; the old code released the claim
// and left the row FAILED, so the next plain --resume created it.
func TestRolledBackRetryOfALegacyFailedRowIsNotCreatedLater(t *testing.T) {
	testenv.Isolate(t)
	s := newDLStub(t)
	quickReconcile(t, s)
	s.taskEntities = []string{"a@contoso.com"}
	const key = "20261015-201500-old"
	writeLegacyLedger(t, key, []downloadRow{{Entity: "a@contoso.com", SubSource: "O365MAIL", SnapshotRaw: 20221231042000,
		Status: dlFailed, Detail: "portal: startRestoreOrDownload: status 502", Label: rowLabelPrefix + "0123456789abcdef"}})
	ctx := context.Background()
	g := testGuard(t)
	if err := g.seedFromLedgers(ctx); err != nil {
		t.Fatal(err)
	}
	b, err := loadBatch(key)
	if err != nil {
		t.Fatal(err)
	}
	no := false
	b.IncludeDeleted, b.IncludeLegalHold = &no, &no
	if err := g.narrowFlags(ctx, b); err != nil {
		t.Fatal(err)
	}
	path, _ := batchPath(key)
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(path, "blocker"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := runDownloadBatch(ctx, quietCmd(), openStubSession(t, s), g, b, pendingDownloadRows(b, true, true), 1, true, false); err == nil {
		t.Fatal("a resend whose ledger cannot be saved must report it was not sent")
	}
	if err := os.RemoveAll(path); err != nil {
		t.Fatal(err)
	}
	if err := saveBatch(b); err != nil {
		t.Fatal(err)
	}
	_, _, _ = runDL(t, s, "download", "submit", "--batch", key, "--resume", "--yes")
	_, _, _ = runDL(t, s, downloadSubmitArgs("--sub-sources", "O365MAIL")...)
	if n := len(s.createdAccounts()); n != 0 {
		t.Fatalf("creates = %d, want 0: the legacy create may exist and nothing proved otherwise", n)
	}
}

// Round 6 P1-3/P1-4: a brute-force oracle for EMPTY. Every small world is
// enumerated: two or three rows, each sent in this run or earlier, each
// create accepted, answered-but-lost or refused, and each one dropped,
// created empty, created with files, or created late (not yet counted or
// listed when the run looks; an earlier row's late job lands inside this
// run's window). Outside the batch: nothing, or one download or restore
// that was in flight or unknown at the window's open, changed inside it,
// was accepted just before it, or could not be read, each adding no job or
// one job to the count. The model assumes what reconcile already relies
// on: an accepted create is counted within reconcileDelay, so an earlier
// row's job lands inside the window only if it was accepted just before.
//
// The count and the listing are simulated, emptyRows decides, and
// reconcile's rule applies it to every unfound REQUESTED row. The oracle
// knows each create's truth: EMPTY may only land on a row that was really
// created empty. And the other direction: with no outside activity and
// every row sent now, accepted and created empty, every row is EMPTY.
func TestEmptyInferenceAgainstABruteForceOracle(t *testing.T) {
	const (
		accepted = iota
		lost
		refused
	)
	const (
		dropped = iota
		createdEmpty
		withFiles
		late
		// lateAfterLag: an earlier row accepted more than reconcileDelay
		// before the window (not yet stale) whose job is counted inside
		// it. The guard's lag clause does not see it; only the batch's
		// own REQUESTED row (and the fresh-claim clause) can.
		lateAfterLag
	)
	const (
		noOther = iota
		dlInFlightAtOpen
		dlUnknownAtOpen
		dlChangedInWindow
		dlAcceptedJustBefore
		rsSendingAtOpen
		rsUnknownAtOpen
		rsChangedInWindow
		guardUnreadable
		otherKinds
	)
	type pRow struct {
		now          bool
		resp, effect int
	}
	var shapes []pRow
	for _, now := range []bool{true, false} {
		for resp := accepted; resp <= refused; resp++ {
			for effect := dropped; effect <= lateAfterLag; effect++ {
				if resp == refused && effect != dropped {
					continue
				}
				shapes = append(shapes, pRow{now, resp, effect})
			}
		}
	}
	worlds, violations, positives := 0, 0, 0
	check := func(rows []pRow, other, adds int) {
		worlds++
		b := &downloadBatch{}
		var w countWindow
		w.counted = true
		w.delta = adds
		switch other {
		case dlInFlightAtOpen, dlUnknownAtOpen, dlAcceptedJustBefore, rsSendingAtOpen, rsUnknownAtOpen:
			w.outsideUnsettled = true
		case dlChangedInWindow, rsChangedInWindow:
			w.outsideChanged = true
		case guardUnreadable:
			w.unreadable = true
		}
		for _, r := range rows {
			row := downloadRow{}
			switch r.resp {
			case accepted:
				row.Status, row.AcceptedAt = dlRequested, "2026-10-20T00:00:00Z"
			case lost:
				row.Status = dlUnknown
			case refused:
				row.Status = dlFailed
			}
			b.Rows = append(b.Rows, row)
			created := r.resp != refused && r.effect != dropped
			switch {
			case !created:
			case r.now && (r.effect == createdEmpty || r.effect == withFiles):
				w.delta++ // counted inside the window
			case !r.now && r.effect == late:
				w.delta++                 // an earlier create landing inside the window
				w.outsideUnsettled = true // its claim: UNKNOWN, or accepted just before
			case !r.now && r.effect == lateAfterLag:
				w.delta++ // counted late; worst case, no guard flag
			}
			if !r.now && r.resp == lost {
				w.outsideUnsettled = true // an UNKNOWN claim outside this run
			}
		}
		var sent []*downloadRow
		for i, r := range rows {
			if r.now {
				sent = append(sent, &b.Rows[i])
			}
		}
		empty := emptyRows(b, sent, w)
		allEmptyNow := other == noOther
		for i, r := range rows {
			found := r.resp != refused && r.effect == withFiles
			markedEmpty := b.Rows[i].Status == dlRequested && !found && empty[&b.Rows[i]]
			reallyEmpty := r.resp != refused && r.effect == createdEmpty
			if markedEmpty && !reallyEmpty {
				violations++
				if violations <= 5 {
					t.Errorf("row %d marked EMPTY but it was %+v; world rows=%+v other=%d adds=%d", i, r, rows, other, adds)
				}
			}
			if !(r.now && r.resp == accepted && r.effect == createdEmpty) {
				allEmptyNow = false
			}
		}
		if allEmptyNow {
			positives++
			for i := range rows {
				if b.Rows[i].Status != dlRequested || !empty[&b.Rows[i]] {
					t.Errorf("a quiet world where every row was sent now and created empty must reach EMPTY: rows=%+v", rows)
					break
				}
			}
		}
	}
	for n := 2; n <= 3; n++ {
		idx := make([]int, n)
		for {
			rows := make([]pRow, n)
			for i, k := range idx {
				rows[i] = shapes[k]
			}
			for other := noOther; other < otherKinds; other++ {
				for adds := 0; adds <= 1; adds++ {
					if other == noOther && adds == 1 {
						continue
					}
					check(rows, other, adds)
				}
			}
			i := 0
			for ; i < n; i++ {
				idx[i]++
				if idx[i] < len(shapes) {
					break
				}
				idx[i] = 0
			}
			if i == n {
				break
			}
		}
	}
	if positives == 0 {
		t.Fatal("the enumeration has no quiet all-empty world: the positive direction is untested")
	}
	if violations > 0 {
		t.Fatalf("%d of %d worlds put EMPTY on a row that was not created empty", violations, worlds)
	}
	t.Logf("%d worlds, %d quiet all-empty worlds reached EMPTY, 0 violations", worlds, positives)
}

// P1-3, the reported case: A is accepted (never created) and B refused,
// with --no-reconcile. The resume sends only B, which creates one empty
// job. The old rule counted only B against the +1 but applied EMPTY to
// every REQUESTED row, so A went EMPTY too and the run exited 0 with A's
// archive silently missing. A was not sent in this run: it is MISSING.
func TestEmptyIsNeverInferredForARowNotSentInThisRun(t *testing.T) {
	testenv.Isolate(t)
	s := newDLStub(t)
	quickReconcile(t, s)
	s.mu.Lock()
	s.listCreated = false
	s.taskEntities = []string{"a@contoso.com", "b@contoso.com"}
	s.createCode["b@contoso.com"] = http.StatusForbidden
	s.mu.Unlock()
	_, _, _ = runDL(t, s, downloadSubmitArgs("--sub-sources", "O365MAIL", "--no-reconcile")...)
	key := ledgerKeys(t)[0]
	s.mu.Lock()
	s.createCode["b@contoso.com"] = 0
	s.onDashboard = func(call int) {
		if call == 3 { // the resume's second read: B made one empty job
			s.dashboardExtra = 1
		}
	}
	s.mu.Unlock()
	_, stderr, err := runDL(t, s, "download", "submit", "--batch", key, "--resume", "--yes")
	if n := len(s.createdAccounts()); n != 3 {
		t.Fatalf("creates = %d, want 3 (A, B refused, B again)", n)
	}
	if err == nil || ExitCode(err) != 6 {
		t.Fatalf("A was never created, so the batch is unsettled: want exit 6, got %v\n%s", err, stderr)
	}
	if r := rowByEntity(mustLoadBatch(t, key), "a@contoso.com"); r.Status == dlEmpty {
		t.Fatalf("A was not sent in this run and must not be inferred EMPTY: %+v", r)
	}
}

// P1-4: an outside create that was already UNKNOWN (or a restore already
// in flight) before the window, and never updated inside it, lands its job
// in the window while this run's create is dropped. The old rule only
// looked for claims updated inside the window, saw none, and marked the
// dropped row EMPTY.
func TestEmptyIsNotInferredWhileAnOutsideCreateIsUnsettled(t *testing.T) {
	old := isoUTC(time.Now().Add(-time.Hour))
	for _, tc := range []struct {
		name  string
		setup func(t *testing.T)
	}{
		{"download UNKNOWN", func(t *testing.T) {
			g := testGuard(t)
			insertClaim(t, g, "O365NOTES", dgUnknown, "20221231-000000-other", "", "")
			if _, err := g.db.Exec(`UPDATE download_claims SET updated_at = ? WHERE sub_source = 'O365NOTES'`, old); err != nil {
				t.Fatal(err)
			}
		}},
		{"restore SENDING", func(t *testing.T) {
			rg, err := openRestoreGuard(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			defer rg.Close()
			if _, err := rg.db.Exec(`INSERT INTO restore_claims (task_id, entity, resolved_raw, state, batch_id, owner_run, created_at, updated_at)
				VALUES ('task-9', 'z@contoso.com', 1, ?, 'r-1', 'other-run', ?, ?)`, claimSending, old, old); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testenv.Isolate(t)
			s := newDLStub(t)
			quickReconcile(t, s)
			s.mu.Lock()
			s.listCreated = false // this run's create is dropped
			s.taskEntities = []string{"a@contoso.com"}
			s.onDashboard = func(call int) {
				if call == 2 { // the outside create lands inside the window
					s.dashboardExtra = 1
				}
			}
			s.mu.Unlock()
			tc.setup(t)
			_, stderr, err := runDL(t, s, downloadSubmitArgs("--sub-sources", "O365MAIL")...)
			if err == nil || ExitCode(err) != 6 {
				t.Fatalf("want exit 6 (MISSING), got %v\n%s", err, stderr)
			}
			if r := mustLoadBatch(t, ledgerKeys(t)[0]).Rows[0]; r.Status != dlMissing {
				t.Fatalf("a dropped create must be MISSING while outside work was unsettled, got %s", r.Status)
			}
		})
	}
}

// countActivity, row by row: what outside this run can and cannot move
// the count.
func TestCountActivitySeesOnlyWhatCanMoveTheCount(t *testing.T) {
	testenv.Isolate(t)
	g := testGuard(t)
	ctx := context.Background()
	since := time.Now()
	set := func(state, jobID, owner string, updated time.Time) {
		t.Helper()
		if _, err := g.db.Exec(`DELETE FROM download_claims`); err != nil {
			t.Fatal(err)
		}
		insertClaim(t, g, "O365MAIL", state, "20221231-000000-other", owner, "")
		if _, err := g.db.Exec(`UPDATE download_claims SET job_id = ?, created_at = ?, updated_at = ?`, jobID, isoUTC(updated), isoUTC(updated)); err != nil {
			t.Fatal(err)
		}
	}
	long := since.Add(-2 * time.Hour)
	for _, tc := range []struct {
		name               string
		state, job, owner  string
		updated            time.Time
		unsettled, changed bool
	}{
		{"unknown long ago", dgUnknown, "", "other", long, true, false},
		{"in flight", dgInFlight, "", "other", long, true, false},
		{"accepted long ago (stale)", dgCreated, "", "other", long, false, false},
		// Round 7 runtime P2-1: accepted, not confirmed, not yet stale:
		// its job may still be counted late.
		{"accepted 30 minutes ago, unconfirmed", dgCreated, "", "other", since.Add(-30 * time.Minute), true, false},
		{"accepted just before", dgCreated, "", "other", since.Add(-time.Second), true, false},
		{"settled in the window", dgCreated, "", "other", since.Add(2 * time.Second), true, true},
		{"refused in the window", dgRejected, "", "other", since.Add(2 * time.Second), true, true},
		// Round 7 P1-a: another run can create and pin a job entirely
		// inside the window; a confirmed claim is activity too.
		{"confirmed in the window", dgCreated, "job-1", "other", since.Add(2 * time.Second), true, true},
		{"confirmed just before", dgCreated, "job-1", "other", since.Add(-time.Second), true, false},
		{"confirmed long ago", dgCreated, "job-1", "other", long, false, false},
		{"this run's own", dgUnknown, "", g.run, since.Add(2 * time.Second), false, false},
	} {
		set(tc.state, tc.job, tc.owner, tc.updated)
		u, c, err := g.countActivity(ctx, "acct-1", since, 10*time.Second)
		if err != nil || u != tc.unsettled || c != tc.changed {
			t.Errorf("%s: unsettled=%v changed=%v err=%v, want %v %v", tc.name, u, c, err, tc.unsettled, tc.changed)
		}
		if u, c, _ := g.countActivity(ctx, "acct-other", since, 10*time.Second); u || c {
			t.Errorf("%s: another account's claim moved this account's count", tc.name)
		}
	}
}

// Docs review 1: submit --dry-run opened the portal, whose handoff POST a
// dry-run client only prints, so it exited 1 before showing anything. It
// now says what a real run would do, sends nothing, and exits 0.
func TestDownloadSubmitDryRunSendsNothing(t *testing.T) {
	testenv.Isolate(t)
	s := newDLStub(t)
	var hits atomic.Int32
	for _, srv := range []*httptest.Server{s.api, s.portal} {
		h := srv.Config.Handler
		srv.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hits.Add(1)
			h.ServeHTTP(w, r)
		})
	}
	out, stderr, err := runDL(t, s, "download", "submit", "--account", "acct-1", "--task", "task-1", "--on-date", "2022-12-31", "--dry-run")
	if err != nil {
		t.Fatalf("--dry-run must exit 0, got %v\n%s\n%s", err, out, stderr)
	}
	if n := hits.Load(); n != 0 {
		t.Fatalf("--dry-run sent %d request(s)", n)
	}
	if !strings.Contains(out, "would resolve the backup on 2022-12-31 (UTC)") || !strings.Contains(out, "EXPORT job per mailbox and sub-source through the customer portal") || strings.Contains(out, "recovery/export") {
		t.Fatalf("--dry-run should say what it would do:\n%s", out)
	}
	if len(ledgerKeys(t)) != 0 {
		t.Fatal("--dry-run saved a ledger")
	}
}

// readyAllJobs makes every listed job ready with one small archive.
func readyAllJobs(s *dlStub) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.jobs {
		j := &s.jobs[i]
		id := "d-" + j.JobID
		j.Status = portal.StatusReadyForDownload
		j.Files = []portal.DownloadFile{{ExportDataID: id, FileName: j.SubSource + ".zip", Size: "6 Bytes"}}
		s.bodies[id] = []byte("PK\x03\x04ok")
	}
}

// Docs review 2: fetch --batch of the whole task after a pilot left the
// pilot's mailbox HELD_BY_OTHER_BATCH, undownloaded, and exited 0. A held
// row is now collected through its holder's confirmed job; while the
// holder has not confirmed it, the row is NOT_READY (exit 8).
func TestFetchBatchCollectsTheArchivesAPilotHolds(t *testing.T) {
	testenv.Isolate(t)
	s := newDLStub(t)
	quickReconcile(t, s)
	s.taskEntities = []string{"a@contoso.com", "b@contoso.com"}
	// The pilot's creates are accepted but not yet confirmed.
	_, _, _ = runDL(t, s, downloadSubmitArgs("--entities", "a@contoso.com", "--no-reconcile")...)
	pilot := ledgerKeys(t)[0]
	if _, stderr, err := runDL(t, s, downloadSubmitArgs()...); err != nil {
		t.Fatalf("the whole task: %v\n%s", err, stderr)
	}
	var whole string
	for _, k := range ledgerKeys(t) {
		if k != pilot {
			whole = k
		}
	}
	readyAllJobs(s)
	heldOnDisk := func(dest string) int {
		n := 0
		_ = filepath.WalkDir(dest, func(p string, d os.DirEntry, err error) error {
			if err == nil && !d.IsDir() && strings.Contains(p, "a@contoso.com") && strings.HasSuffix(p, ".zip") {
				n++
			}
			return nil
		})
		return n
	}

	dest := t.TempDir()
	out, stderr, err := runDL(t, s, fetchArgs(dest, "--batch", whole)...)
	if err == nil || ExitCode(err) != ExitJobPending {
		t.Fatalf("the pilot has not confirmed a@ yet: want exit %d, got %v\n%s\n%s", ExitJobPending, err, out, stderr)
	}
	if !strings.Contains(out, "--batch "+pilot+" --resume --yes") {
		t.Fatalf("the NOT_READY rows should name the pilot's resume:\n%s", out)
	}

	if _, stderr, err := runDL(t, s, "download", "submit", "--batch", pilot, "--resume", "--yes"); err != nil {
		t.Fatalf("pilot resume: %v\n%s", err, stderr)
	}
	readyAllJobs(s)
	dest = t.TempDir()
	if out, stderr, err := runDL(t, s, fetchArgs(dest, "--batch", whole)...); err != nil {
		t.Fatalf("every archive of the whole batch should be on disk: %v\n%s\n%s", err, out, stderr)
	}
	if n := heldOnDisk(dest); n != 2 {
		t.Fatalf("a@'s two archives (held by the pilot) on disk = %d, want 2", n)
	}
}

// The holder's ledger cannot be read: the held row is MISSING, never a
// silent success.
func TestFetchBatchHeldRowWithAnUnreadableHolderIsMissing(t *testing.T) {
	testenv.Isolate(t)
	s := newDLStub(t)
	quickReconcile(t, s)
	s.taskEntities = []string{"a@contoso.com", "b@contoso.com"}
	_, _, _ = runDL(t, s, downloadSubmitArgs("--entities", "a@contoso.com")...)
	pilot := ledgerKeys(t)[0]
	_, _, _ = runDL(t, s, downloadSubmitArgs()...)
	var whole string
	for _, k := range ledgerKeys(t) {
		if k != pilot {
			whole = k
		}
	}
	readyAllJobs(s)
	path, _ := batchPath(pilot)
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, _, err := runDL(t, s, fetchArgs(t.TempDir(), "--batch", whole)...)
	if err == nil || ExitCode(err) != 6 {
		t.Fatalf("want exit 6, got %v\n%s", err, out)
	}
	if !strings.Contains(out, "batch "+pilot+" holds this archive, but its ledger cannot tell") {
		t.Fatalf("the MISSING row should say why:\n%s", out)
	}
}

// Docs review 3: the re-run guard left out the time zone, so a corrected
// run with --utc-offset -5 was refused as "this same command" and told to
// continue the UTC-day batch, which holds the wrong day.
func TestRerunGuardTellsTimeZonesApart(t *testing.T) {
	testenv.Isolate(t)
	mk := func(batchKey, onDate, zone string) *downloadBatch {
		key := rerunIdentity("account", "acct-1", "task-1", onDate, "zip", nil, nil, false, false)
		return &downloadBatch{BatchKey: batchKey, AccountID: "acct-1", TaskID: "task-1", OnDate: onDate, Timezone: zone,
			Format: "zip", RerunKey: key, CreatedAt: isoUTC(time.Now()),
			Rows: []downloadRow{{Entity: "a@contoso.com", SubSource: "O365MAIL", Status: dlCreated, JobID: "job-1"}}}
	}
	if err := saveBatch(mk("20261020-000000-utc", "2022-12-31", "UTC")); err != nil {
		t.Fatal(err)
	}
	if err := claimNewBatch(mk("20261020-000001-east", "2022-12-31", "UTC-5"), false); err != nil {
		t.Fatalf("the same date in UTC-5 is a different command: %v", err)
	}
	if err := claimNewBatch(mk("20261020-000002-again", "2022-12-31", ""), false); err == nil || !strings.Contains(err.Error(), "20261020-000000-utc is this same command") {
		t.Fatalf("the same date in UTC (an empty zone reads as UTC) must still be refused, got %v", err)
	}
	// --latest picks each mailbox's newest backup whatever the zone.
	if err := saveBatch(mk("20261020-000003-latest", latestLabel, "UTC")); err != nil {
		t.Fatal(err)
	}
	b := mk("20261020-000004-latest", latestLabel, "UTC+2")
	if err := claimNewBatch(b, false); err == nil {
		t.Fatal("two --latest runs differing only in --utc-offset are the same command")
	}
}

// Docs review 4: a refused re-run printed "N of these archive(s) ... will
// be left out (SKIPPED); the rest are created" before the re-run guard
// refused it, so an agent could report creates that never happened.
func TestRefusedRerunPrintsNoSkippedNotice(t *testing.T) {
	testenv.Isolate(t)
	s := newDLStub(t)
	quickReconcile(t, s)
	s.taskEntities = []string{"a@contoso.com", "b@contoso.com"}
	if _, stderr, err := runDL(t, s, downloadSubmitArgs("--entities", "a@contoso.com")...); err != nil {
		t.Fatalf("pilot: %v\n%s", err, stderr)
	}
	if _, stderr, err := runDL(t, s, downloadSubmitArgs()...); err != nil || !strings.Contains(stderr, "left out here (SKIPPED)") {
		t.Fatalf("the whole task should say what it leaves out once it is recorded: %v\n%s", err, stderr)
	}
	_, stderr, err := runDL(t, s, downloadSubmitArgs()...)
	if err == nil || ExitCode(err) != 2 || !strings.Contains(stderr+err.Error(), "is this same command") {
		t.Fatalf("the same command again must be refused, got %v\n%s", err, stderr)
	}
	if strings.Contains(stderr, "SKIPPED") {
		t.Fatalf("a refused run must not report archives left out:\n%s", stderr)
	}
}

// P1-g: the first build saved every row PLANNED before its creates and
// again only at the end, so a run stopped mid-way left PLANNED rows whose
// jobs exist. Seeding skipped them, a new batch created them again, the
// legacy refusal offered them as never created, and a legacy resume sent
// them without looking. A legacy row is now never treated as unsent.
func TestLegacyPlannedRowsAreNeverTreatedAsUnsent(t *testing.T) {
	testenv.Isolate(t)
	s := newDLStub(t)
	quickReconcile(t, s)
	s.mu.Lock()
	s.taskEntities = []string{"a@contoso.com"}
	// The killed legacy run's create of a@'s mail did reach the portal.
	s.jobs = append(s.jobs, dlJob{JobID: "job-legacy", Account: "a@contoso.com", SubSource: "O365MAIL", Status: "INPROCESS",
		Snapshot: 20221231042000, Created: compactAt(time.Now().Add(-time.Hour))})
	s.mu.Unlock()
	const key = "20261015-201500-old"
	writeLegacyLedger(t, key, []downloadRow{
		{Entity: "a@contoso.com", SubSource: "O365MAIL", SnapshotRaw: 20221231042000, Status: dlPlanned},
		{Entity: "a@contoso.com", SubSource: "O365CALENDAR", SnapshotRaw: 20221231041000, Status: dlPlanned},
	})

	_, stderr, err := runDL(t, s, downloadSubmitArgs("--entities", "a@contoso.com")...)
	if err == nil || ExitCode(err) != 2 {
		t.Fatalf("a new batch must be refused while legacy PLANNED rows may exist, got %v\n%s", err, stderr)
	}
	_, stderr, _ = runDL(t, s, "download", "submit", "--batch", key, "--resume", "--yes")
	if strings.Contains(stderr, "never created") {
		t.Fatalf("legacy PLANNED rows must not be offered as never created:\n%s", stderr)
	}
	_, stderr, _ = runDL(t, s, "download", "submit", "--batch", key, "--resume", "--include-deleted=false", "--include-legal-hold=false", "--yes")
	if n := len(s.createdAccounts()); n != 0 {
		t.Fatalf("creates = %d, want 0: a legacy PLANNED row may already exist\n%s", n, stderr)
	}
	b := mustLoadBatch(t, key)
	for _, r := range b.Rows {
		if r.Status != dlUnknown {
			t.Fatalf("legacy PLANNED rows should be held UNKNOWN after the resume looked: %+v", r)
		}
	}
	if !b.Rows[0].Ambiguous {
		t.Fatalf("a@'s mail has a listed job that cannot be proved its own: %+v", b.Rows[0])
	}
}

// P1-h: download start sent the operator's --label as the job label, and
// matchRow took that label as proof. Two starts of one mailbox and date
// with the same --label but different include flags both pinned the first
// labelled job, so the legal-hold batch was confirmed to the plain export.
// Each start now sends a unique tool label with the text after it, and a
// job is matched by the unique part alone.
func TestTwoStartsWithTheSameLabelPinDifferentJobs(t *testing.T) {
	ent := taskEntity{Entity: "a@contoso.com"}
	row := planRow{ResolvedRaw: 20221231042000}
	scope := restoreScope{account: "acct-1"}
	plain := startBatch(scope, "task-1", "MS365", ent, "O365MAIL", "2022-12-31", row, "zip", "year end", false, false)
	hold := startBatch(scope, "task-1", "MS365", ent, "O365MAIL", "2022-12-31", row, "zip", "year end", false, true)
	p, h := &plain.Rows[0], &hold.Rows[0]
	now := time.Now()
	p.RequestedAt, h.RequestedAt = isoUTC(now), isoUTC(now)
	job := func(id, label string) portal.DownloadJob {
		return portal.DownloadJob{JobID: id, Account: "a@contoso.com", SubSource: "O365MAIL", SnapshotDate: 20221231042000,
			JobLabel: label, InitDate: compactAt(now.Add(time.Minute))}
	}
	// The portal lists each job with the label its create sent.
	jobs := []portal.DownloadJob{job("job-plain", p.Label), job("job-hold", h.Label)}
	mp := matchRow(p, jobs, "zip", true, nil)
	mh := matchRow(h, jobs, "zip", true, nil)
	if !mp.found || !mh.found || mp.job.JobID != "job-plain" || mh.job.JobID != "job-hold" {
		t.Fatalf("each start must pin its own job: plain=%+v hold=%+v", mp, mh)
	}
	if p.Label == h.Label || !strings.HasSuffix(p.Label, " year end") || rowLabelKey(p.Label) == "" {
		t.Fatalf("labels %q and %q: want unique tool labels ending in the operator's text", p.Label, h.Label)
	}
	// The operator's text cut short (or edited) on the portal: still found.
	cut := []portal.DownloadJob{job("job-hold", rowLabelKey(h.Label)+" year")}
	if m := matchRow(h, cut, "zip", true, nil); !m.found {
		t.Fatalf("the unique part alone must match: %+v", m)
	}
	if long := startLabel(strings.Repeat("x", 500)); len(long) != maxJobLabel || rowLabelKey(long) == "" {
		t.Fatalf("a long --label is cut after the unique part: %q", long)
	}
}

// P2-f: after --no-reconcile (or an interrupted wait, or download start) a
// resume with nothing to send ran only the confirming pass, so a dropped
// create stayed REQUESTED for good. A REQUESTED row accepted over an hour
// ago that a fresh listing still does not show is MISSING (never EMPTY),
// and that resume sends it again. The clock is moved by writing the
// accepted times into the ledger.
func TestStaleRequestedRowsBecomeMissing(t *testing.T) {
	testenv.Isolate(t)
	s := newDLStub(t)
	quickReconcile(t, s)
	s.mu.Lock()
	s.listCreated = false
	s.taskEntities = []string{"a@contoso.com", "b@contoso.com"}
	s.mu.Unlock()
	_, _, _ = runDL(t, s, downloadSubmitArgs("--sub-sources", "O365MAIL", "--no-reconcile")...)
	key := ledgerKeys(t)[0]
	b := mustLoadBatch(t, key)
	for i := range b.Rows {
		age := 30 * time.Minute
		if b.Rows[i].Entity == "a@contoso.com" {
			age = 2 * time.Hour
		}
		b.Rows[i].AcceptedAt = isoUTC(time.Now().Add(-age))
	}
	if err := saveBatch(b); err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	s.listCreated = true
	s.mu.Unlock()
	_, stderr, _ := runDL(t, s, "download", "submit", "--batch", key, "--resume", "--yes")
	if got := s.createdAccounts(); len(got) != 3 || got[2] != "a@contoso.com" {
		t.Fatalf("creates = %v, want a@ sent again once (its create was dropped an hour ago)\n%s", got, stderr)
	}
	if r := rowByEntity(mustLoadBatch(t, key), "a@contoso.com"); r.Status != dlCreated {
		t.Fatalf("the stale row was sent again and found: %+v", r)
	}

	// The confirming pass alone, with no job listed: only the row
	// accepted over an hour ago is MISSING.
	b.Rows[0].Status, b.Rows[1].Status = dlRequested, dlRequested
	if _, err := reconcileRows(context.Background(), testGuard(t), b, nil, false, true, nil); err != nil {
		t.Fatal(err)
	}
	if a, bb := rowByEntity(b, "a@contoso.com"), rowByEntity(b, "b@contoso.com"); a.Status != dlMissing || bb.Status != dlRequested {
		t.Fatalf("want a@ MISSING (2h) and b@ REQUESTED (30m), got %s and %s", a.Status, bb.Status)
	}
}

// P2-h: a run that sent nothing waited the whole reconcile delay for jobs
// that cannot exist.
func TestARunThatSentNothingDoesNotWait(t *testing.T) {
	testenv.Isolate(t)
	s := newDLStub(t)
	prev := reconcileDelay
	reconcileDelay = 3 * time.Second
	t.Cleanup(func() { reconcileDelay = prev })
	s.taskEntities = []string{"a@contoso.com"}
	s.createCode["a@contoso.com"] = http.StatusForbidden
	start := time.Now()
	_, stderr, err := runDL(t, s, downloadSubmitArgs("--sub-sources", "O365MAIL")...)
	if took := time.Since(start); took > 2*time.Second {
		t.Fatalf("a run whose only create was refused took %s: it waited to reconcile\n%s", took, stderr)
	}
	if err == nil || ExitCode(err) != 6 {
		t.Fatalf("a refused create leaves the row FAILED: want exit 6, got %v", err)
	}
}

// P2-g: fetch --batch with another customer's --account listed that
// customer's jobs and called every confirmed archive expired, printing
// --new-batch commands. A batch is only run for the account it was
// recorded for; submit --resume checks the same.
func TestBatchRunsAreRefusedForAnotherAccount(t *testing.T) {
	testenv.Isolate(t)
	s := newDLStub(t)
	quickReconcile(t, s)
	s.taskEntities = []string{"a@contoso.com"}
	if _, stderr, err := runDL(t, s, downloadSubmitArgs("--sub-sources", "O365MAIL")...); err != nil {
		t.Fatalf("submit: %v\n%s", err, stderr)
	}
	key := ledgerKeys(t)[0]
	for _, args := range [][]string{
		{"download", "fetch", "--account", "acct-2", "--dest", t.TempDir(), "--year", "2022", "--batch", key},
		{"download", "submit", "--account", "acct-2", "--batch", key, "--resume", "--yes"},
	} {
		out, stderr, err := runDL(t, s, args...)
		if err == nil || ExitCode(err) != 2 || !strings.Contains(err.Error(), "Run it with --account acct-1") {
			t.Fatalf("%s for another account: want exit 2 naming --account acct-1, got %v\n%s\n%s", args[1], err, out, stderr)
		}
		if strings.Contains(out+stderr, "--new-batch") {
			t.Fatalf("%s must not suggest --new-batch:\n%s\n%s", args[1], out, stderr)
		}
	}
	if n := len(s.createdAccounts()); n != 1 {
		t.Fatalf("creates = %d, want 1", n)
	}
}

// A --new-batch create that is cleanly refused (or never sent, or rolled
// back) puts back the claim it superseded, exactly as it was, in the same
// transaction. The old code left the original batch's CREATED claim
// superseded and the new one REJECTED, so the archive was free and a later
// plain run created it a second time. An UNKNOWN --new-batch create keeps
// the archive held by its own claim.
func TestRefusedNewBatchCreatePutsTheOriginalClaimBack(t *testing.T) {
	for _, tc := range []struct {
		name string
		code int
	}{{"refused 403", http.StatusForbidden}, {"unknown 502", http.StatusBadGateway}} {
		t.Run(tc.name, func(t *testing.T) {
			testenv.Isolate(t)
			s := newDLStub(t)
			quickReconcile(t, s)
			s.taskEntities = []string{"a@contoso.com"}
			if _, stderr, err := runDL(t, s, downloadSubmitArgs("--sub-sources", "O365MAIL")...); err != nil {
				t.Fatalf("original: %v\n%s", err, stderr)
			}
			original := ledgerKeys(t)[0]
			state, batch, jobID, _, _ := guardClaimRow(t, testGuard(t), "O365MAIL")
			if state != dgCreated || jobID == "" || batch != original {
				t.Fatalf("the original should hold a confirmed claim: %s %s %s", state, batch, jobID)
			}
			s.mu.Lock()
			s.createCode["a@contoso.com"] = tc.code
			s.mu.Unlock()
			_, _, _ = runDL(t, s, downloadSubmitArgs("--sub-sources", "O365MAIL", "--new-batch")...)
			var newer string
			for _, k := range ledgerKeys(t) {
				if k != original {
					newer = k
				}
			}
			s.mu.Lock()
			s.createCode["a@contoso.com"] = 0
			s.mu.Unlock()
			_, _, _ = runDL(t, s, downloadSubmitArgs("--sub-sources", "O365MAIL", "--entities", "a@contoso.com")...)
			_, _, _ = runDL(t, s, "download", "submit", "--batch", original, "--resume", "--yes")
			_, _, _ = runDL(t, s, "download", "submit", "--batch", newer, "--resume", "--yes")
			if n := len(s.createdAccounts()); n != 2 {
				t.Fatalf("creates = %d, want 2 (the original and the %s --new-batch attempt)", n, tc.name)
			}
			if tc.code == http.StatusForbidden {
				var superseded int
				var st, bk, job string
				if err := testGuard(t).db.QueryRow(`SELECT state, batch_key, job_id, superseded FROM download_claims WHERE batch_key = ?`, original).Scan(&st, &bk, &job, &superseded); err != nil {
					t.Fatal(err)
				}
				if st != state || bk != batch || job != jobID || superseded != 0 {
					t.Fatalf("the original claim should be back exactly: %s %s %s superseded=%d, want %s %s %s", st, bk, job, superseded, state, batch, jobID)
				}
			}
		})
	}
}

// The same for a --new-batch create rolled back because its ledger could
// not be saved: nothing was sent, and the original claim is back.
func TestRolledBackNewBatchCreatePutsTheOriginalClaimBack(t *testing.T) {
	testenv.Isolate(t)
	s := newDLStub(t)
	quickReconcile(t, s)
	s.taskEntities = []string{"a@contoso.com"}
	if _, stderr, err := runDL(t, s, downloadSubmitArgs("--sub-sources", "O365MAIL")...); err != nil {
		t.Fatalf("original: %v\n%s", err, stderr)
	}
	original := ledgerKeys(t)[0]
	nb := mustLoadBatch(t, original)
	nb.BatchKey = "20261020-000000-acct-newbatch"
	nb.Rows[0].Status, nb.Rows[0].JobID, nb.Rows[0].Label, nb.Rows[0].ConfirmedAt = dlPlanned, "", "", ""
	path, _ := batchPath(nb.BatchKey)
	if err := os.MkdirAll(filepath.Join(path, "blocker"), 0o700); err != nil {
		t.Fatal(err)
	}
	g := testGuard(t)
	if err := runDownloadBatch(context.Background(), quietCmd(), openStubSession(t, s), g, nb, pendingDownloadRows(nb, false, false), 1, false, true); err == nil {
		t.Fatal("a create whose ledger cannot be saved must report it was not sent")
	}
	if n := len(s.createdAccounts()); n != 1 {
		t.Fatalf("creates = %d, want 1: the rolled-back create was not sent", n)
	}
	var st string
	var superseded int
	if err := g.db.QueryRow(`SELECT state, superseded FROM download_claims WHERE batch_key = ?`, original).Scan(&st, &superseded); err != nil {
		t.Fatal(err)
	}
	if st != dgCreated || superseded != 0 {
		t.Fatalf("the original claim should be back: %s superseded=%d", st, superseded)
	}
}

// Round 7 P1-a, through the real guard SQL: while this run waits, another
// run creates a job and pins it, entirely between the two count reads. The
// count grows by one, this run's own create was dropped, and the old
// activity check skipped confirmed claims, so the dropped row went EMPTY.
func TestEmptyIsNotInferredWhenAnotherRunCreatesAndPinsInTheWindow(t *testing.T) {
	testenv.Isolate(t)
	s := newDLStub(t)
	quickReconcile(t, s)
	g := testGuard(t)
	s.mu.Lock()
	s.listCreated = false // this run's create is dropped
	s.taskEntities = []string{"a@contoso.com"}
	s.onDashboard = func(call int) {
		if call != 2 {
			return
		}
		s.dashboardExtra = 1
		insertClaim(t, g, "O365NOTES", dgCreated, "20221231-000000-other", "other-run", rowLabelPrefix+"00000000000000e1")
		if _, err := g.db.Exec(`UPDATE download_claims SET job_id = 'job-other' WHERE sub_source = 'O365NOTES'`); err != nil {
			t.Error(err)
		}
	}
	s.mu.Unlock()
	_, stderr, err := runDL(t, s, downloadSubmitArgs("--sub-sources", "O365MAIL")...)
	if err == nil || ExitCode(err) != 6 {
		t.Fatalf("want exit 6 (MISSING), got %v\n%s", err, stderr)
	}
	if r := mustLoadBatch(t, ledgerKeys(t)[0]).Rows[0]; r.Status != dlMissing {
		t.Fatalf("a dropped create must be MISSING when another run created and pinned a job in the window, got %s", r.Status)
	}
}

// Round 7 P1-b: DownloadJobs stopped after 50 pages of 100 and reported
// success, so a job beyond the first 5,000 read as absent. The stale rule
// then made its row MISSING and the resume created the export again, and
// fetch called the confirmed archive expired. A listing cut at its cap is
// now marked incomplete, and nothing is concluded from absence in it.
func TestACutJobListingConcludesNothingFromAbsence(t *testing.T) {
	testenv.Isolate(t)
	unpacedPortal(t)
	s := newDLStub(t)
	quickReconcile(t, s)
	s.mu.Lock()
	s.listCreated = false
	s.taskEntities = []string{"a@contoso.com"}
	s.mu.Unlock()
	_, _, _ = runDL(t, s, downloadSubmitArgs("--sub-sources", "O365MAIL", "--no-reconcile")...)
	key := ledgerKeys(t)[0]
	b := mustLoadBatch(t, key)
	b.Rows[0].AcceptedAt = isoUTC(time.Now().Add(-2 * time.Hour))
	if err := saveBatch(b); err != nil {
		t.Fatal(err)
	}
	// More jobs than the listing's cap; this row's own job lies beyond it.
	s.mu.Lock()
	for i := 0; i < 5000; i++ {
		s.jobs = append(s.jobs, dlJob{JobID: fmt.Sprintf("other-%d", i), Account: "z@contoso.com", SubSource: "O365MAIL",
			Status: "INPROCESS", Snapshot: 20200101000000, Created: compactNow()})
	}
	s.jobs = append(s.jobs, dlJob{JobID: "job-mine", Account: "a@contoso.com", SubSource: "O365MAIL", Status: "INPROCESS",
		Snapshot: b.Rows[0].SnapshotRaw, Label: b.Rows[0].Label, Created: compactNow()})
	s.mu.Unlock()

	_, stderr, err := runDL(t, s, "download", "submit", "--batch", key, "--resume", "--yes")
	if n := len(s.createdAccounts()); n != 1 {
		t.Fatalf("creates = %d, want 1: the row's job exists beyond the listing's cap\n%s", n, stderr)
	}
	if err == nil || ExitCode(err) != 6 || !strings.Contains(stderr, "page cap") {
		t.Fatalf("a resume over a cut listing must say so and exit 6, got %v\n%s", err, stderr)
	}
	if r := mustLoadBatch(t, key).Rows[0]; r.Status != dlRequested {
		t.Fatalf("absence from a cut listing must leave the row as it was, got %s", r.Status)
	}

	// fetch of a confirmed row whose job is beyond the cut: UNLISTED, not
	// MISSING with a --new-batch command.
	b = mustLoadBatch(t, key)
	b.Rows[0].Status, b.Rows[0].JobID, b.Rows[0].ConfirmedAt = dlCreated, "job-mine", isoUTC(time.Now())
	if err := saveBatch(b); err != nil {
		t.Fatal(err)
	}
	out, stderr, err := runDL(t, s, fetchArgs(t.TempDir(), "--batch", key)...)
	if err == nil || ExitCode(err) != 6 || !strings.Contains(out, fetchUnlisted) || strings.Contains(out+stderr, "--new-batch") {
		t.Fatalf("want UNLISTED, exit 6 and no --new-batch, got %v\n%s\n%s", err, out, stderr)
	}
	// watch and list say the listing was cut instead of "not found" or a
	// clean exit.
	if _, _, err := runDL(t, s, "download", "watch", "--account", "acct-1", "--job", "job-mine", "--once"); err == nil || ExitCode(err) != 6 || !strings.Contains(err.Error(), "may still exist") {
		t.Fatalf("watch of a job beyond a cut listing: want exit 6 saying it may exist, got %v", err)
	}
	if _, stderr, err := runDL(t, s, "download", "list", "--account", "acct-1", "--json"); err == nil || ExitCode(err) != 6 || !strings.Contains(stderr, "page cap") {
		t.Fatalf("list of a cut listing: want exit 6 and a warning, got %v\n%s", err, stderr)
	}
}

// unpacedPortal turns off the portal session's call pacing, so a test that
// reads 50 listing pages does not wait 12 seconds for each listing.
func unpacedPortal(t *testing.T) {
	orig := openPortal
	openPortal = func(ctx context.Context, cfg portal.Config, token string) (*portal.Session, error) {
		cfg.RatePerSec = -1
		return orig(ctx, cfg, token)
	}
	t.Cleanup(func() { openPortal = orig })
}

// Round 7 P1-c: pilot P confirms J1, the whole-task batch W holds it as
// SKIPPED, J1 fails, and a --new-batch run creates J2. fetch --batch W used
// to mark J1 as seen, then suppress its failure as replaced, so the held
// archive was neither downloaded nor reported and fetch exited 0. Under
// --batch every row ends with an outcome: a replacement the guard proves
// is downloaded in J1's place, and one not yet confirmed is NOT_READY.
func TestFetchBatchFollowsOnlyAProvenReplacementOfAFailedHeldJob(t *testing.T) {
	for _, mode := range []string{"confirmed", "unconfirmed", "none"} {
		t.Run("replacement "+mode, func(t *testing.T) {
			confirmed := mode == "confirmed"
			testenv.Isolate(t)
			s := newDLStub(t)
			quickReconcile(t, s)
			s.taskEntities = []string{"a@contoso.com", "b@contoso.com"}
			if _, stderr, err := runDL(t, s, downloadSubmitArgs("--entities", "a@contoso.com", "--sub-sources", "O365MAIL")...); err != nil {
				t.Fatalf("pilot: %v\n%s", err, stderr)
			}
			pilot := ledgerKeys(t)[0]
			j1 := mustLoadBatch(t, pilot).Rows[0].JobID
			if _, stderr, err := runDL(t, s, downloadSubmitArgs("--sub-sources", "O365MAIL")...); err != nil {
				t.Fatalf("whole task: %v\n%s", err, stderr)
			}
			var whole string
			for _, k := range ledgerKeys(t) {
				if k != pilot {
					whole = k
				}
			}
			s.mu.Lock()
			for i := range s.jobs {
				if s.jobs[i].JobID == j1 {
					s.jobs[i].Status = "FAILED"
				}
			}
			s.listCreated = confirmed
			s.mu.Unlock()
			if mode != "none" {
				_, _, _ = runDL(t, s, downloadSubmitArgs("--entities", "a@contoso.com", "--sub-sources", "O365MAIL", "--new-batch")...)
			}
			readyAllJobs(s)
			s.mu.Lock()
			for i := range s.jobs {
				if s.jobs[i].JobID == j1 {
					s.jobs[i].Status, s.jobs[i].Files = "FAILED", nil
				}
			}
			s.mu.Unlock()

			dest := t.TempDir()
			out, stderr, err := runDL(t, s, fetchArgs(dest, "--batch", whole)...)
			aOnDisk := 0
			_ = filepath.WalkDir(dest, func(p string, d os.DirEntry, werr error) error {
				if werr == nil && !d.IsDir() && strings.Contains(p, "a@contoso.com") && strings.HasSuffix(p, ".zip") {
					aOnDisk++
				}
				return nil
			})
			if confirmed {
				if err != nil || aOnDisk != 1 {
					t.Fatalf("the proven replacement must be downloaded for the held row: err=%v, a@ archives on disk=%d\n%s\n%s", err, aOnDisk, out, stderr)
				}
				return
			}
			if mode == "none" {
				if err == nil || ExitCode(err) != 6 || !strings.Contains(out, "--batch "+pilot+" --resume --yes") {
					t.Fatalf("a failed held job with no replacement is FAILED with the holder's resume: got %v\n%s", err, out)
				}
				return
			}
			if err == nil || ExitCode(err) != ExitJobPending || !strings.Contains(out, "is creating this archive again") {
				t.Fatalf("an unconfirmed replacement is NOT_READY naming its batch: want exit %d, got %v\n%s\n%s", ExitJobPending, err, out, stderr)
			}
		})
	}
}

// Docs review P2: a Ctrl-C during the first job-count read left every row
// PLANNED and the run exited 0 with "requested 0 archive(s)".
func TestARunStoppedBeforeItsFirstCreateIsUnsettled(t *testing.T) {
	testenv.Isolate(t)
	s := newDLStub(t)
	quickReconcile(t, s)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.mu.Lock()
	s.taskEntities = []string{"a@contoso.com", "b@contoso.com"}
	s.onDashboard = func(call int) {
		if call == 1 {
			cancel()
		}
	}
	s.mu.Unlock()
	_, stderr, err := runDLContext(t, ctx, s, downloadSubmitArgs("--sub-sources", "O365MAIL")...)
	if n := len(s.createdAccounts()); n != 0 {
		t.Fatalf("a stopped run sent %d create(s)", n)
	}
	if err == nil || ExitCode(err) != 6 || !strings.Contains(err.Error(), "2 archive(s) were never sent; continue with: cloudally-cli download submit --batch") {
		t.Fatalf("want exit 6 naming the resume, got %v\n%s", err, stderr)
	}
}

// Docs review P2: a plan with no rows says so plainly (exit 0).
func TestAnEmptyPlanSaysNothingToCreate(t *testing.T) {
	testenv.Isolate(t)
	s := newDLStub(t)
	s.taskEntities = []string{"a@contoso.com"}
	_, stderr, err := runDL(t, s, downloadSubmitArgs("--entities", "nobody@contoso.com")...)
	if err != nil || !strings.Contains(stderr, "nothing to create:") {
		t.Fatalf("want exit 0 and a nothing-to-create notice, got %v\n%s", err, stderr)
	}
}

// Round 7 runtime P1: --resume --no-reconcile skipped the look before
// sending, so a MISSING row whose job had since appeared was created a
// second time. Every resume looks first; --no-reconcile skips only the
// wait after sending.
func TestResumeWithNoReconcileStillLooksBeforeSending(t *testing.T) {
	testenv.Isolate(t)
	s := newDLStub(t)
	quickReconcile(t, s)
	s.mu.Lock()
	s.listCreated = false // accepted, not listed yet: MISSING
	s.taskEntities = []string{"a@contoso.com"}
	s.mu.Unlock()
	_, _, _ = runDL(t, s, downloadSubmitArgs("--sub-sources", "O365MAIL")...)
	key := ledgerKeys(t)[0]
	row := mustLoadBatch(t, key).Rows[0]
	if row.Status != dlMissing {
		t.Fatalf("want MISSING, got %+v", row)
	}
	// The job turns up on the portal later.
	s.mu.Lock()
	s.jobs = append(s.jobs, dlJob{JobID: "job-late", Account: "a@contoso.com", SubSource: "O365MAIL", Status: "INPROCESS",
		Snapshot: row.SnapshotRaw, Label: row.Label, Created: compactNow()})
	s.mu.Unlock()
	_, stderr, err := runDL(t, s, "download", "submit", "--batch", key, "--resume", "--no-reconcile", "--yes")
	if n := len(s.createdAccounts()); n != 1 {
		t.Fatalf("creates = %d, want 1: the MISSING row's job is on the portal now", n)
	}
	if err != nil {
		t.Fatalf("resume: %v\n%s", err, stderr)
	}
	if r := mustLoadBatch(t, key).Rows[0]; r.Status != dlCreated || r.JobID != "job-late" {
		t.Fatalf("the row should be CREATED with its job, got %+v", r)
	}
}

// Round 7 runtime P2-2: the pilot's create of a@ was accepted but never
// appeared (MISSING there). The whole task said the held archive was
// "already created" and fetch then reported it NOT_READY (exit 8, run
// again later), which a script polls forever. Both now say what the
// holder's state is, and fetch exits 6 with the holder's resume.
func TestHeldArchivesAreReportedByTheHoldersState(t *testing.T) {
	testenv.Isolate(t)
	s := newDLStub(t)
	quickReconcile(t, s)
	s.mu.Lock()
	s.listCreated = false
	s.taskEntities = []string{"a@contoso.com", "b@contoso.com"}
	s.mu.Unlock()
	_, _, _ = runDL(t, s, downloadSubmitArgs("--entities", "a@contoso.com", "--sub-sources", "O365MAIL")...)
	pilot := ledgerKeys(t)[0]
	if r := mustLoadBatch(t, pilot).Rows[0]; r.Status != dlMissing {
		t.Fatalf("the pilot's row should be MISSING, got %s", r.Status)
	}
	s.mu.Lock()
	s.listCreated = true
	s.mu.Unlock()
	_, stderr, err := runDL(t, s, downloadSubmitArgs("--sub-sources", "O365MAIL")...)
	if err != nil {
		t.Fatalf("whole task: %v\n%s", err, stderr)
	}
	if strings.Contains(stderr, "already created") || !strings.Contains(stderr, "were accepted by batch "+pilot+" but are not confirmed yet") {
		t.Fatalf("the notice should say the pilot has not confirmed a@:\n%s", stderr)
	}
	var whole string
	for _, k := range ledgerKeys(t) {
		if k != pilot {
			whole = k
		}
	}
	readyAllJobs(s)
	out, _, err := runDL(t, s, fetchArgs(t.TempDir(), "--batch", whole)...)
	if err == nil || ExitCode(err) != 6 || !strings.Contains(out, "its create never appeared on the portal") || !strings.Contains(out, "--batch "+pilot+" --resume --yes") {
		t.Fatalf("a holder-MISSING row is MISSING (exit 6) with the holder's resume, got %v\n%s", err, out)
	}
}

// Round 7 runtime P2-3: fetch --batch of a batch whose rows are all EMPTY
// exited 3, "no download jobs matched". Each row now gets an EMPTY outcome
// and the fetch exits 0.
func TestFetchBatchOfEmptyRowsExitsZero(t *testing.T) {
	testenv.Isolate(t)
	s := newDLStub(t)
	quickReconcile(t, s)
	s.mu.Lock()
	s.listCreated = false
	s.countCreates = true // each create made a job that finished empty
	s.taskEntities = []string{"a@contoso.com"}
	s.mu.Unlock()
	if _, stderr, err := runDL(t, s, downloadSubmitArgs()...); err != nil {
		t.Fatalf("submit: %v\n%s", err, stderr)
	}
	key := ledgerKeys(t)[0]
	for _, r := range mustLoadBatch(t, key).Rows {
		if r.Status != dlEmpty {
			t.Fatalf("want every row EMPTY, got %+v", r)
		}
	}
	out, stderr, err := runDL(t, s, fetchArgs(t.TempDir(), "--batch", key)...)
	if err != nil {
		t.Fatalf("a batch of empty mailboxes is complete: want exit 0, got %v\n%s\n%s", err, out, stderr)
	}
	if n := strings.Count(out, `"status": "EMPTY"`) + strings.Count(out, `"status":"EMPTY"`); n != 2 {
		t.Fatalf("want an EMPTY outcome per row (2), got %d:\n%s", n, out)
	}
}

// Round 7 runtime P2-4: fetch --batch with the wrong --account minted a
// handoff token for that customer and listed its jobs before refusing. The
// batch is now checked before anything reaches the portal.
func TestFetchBatchForAnotherAccountTouchesNoPortal(t *testing.T) {
	testenv.Isolate(t)
	s := newDLStub(t)
	quickReconcile(t, s)
	s.taskEntities = []string{"a@contoso.com"}
	if _, stderr, err := runDL(t, s, downloadSubmitArgs("--sub-sources", "O365MAIL")...); err != nil {
		t.Fatalf("submit: %v\n%s", err, stderr)
	}
	key := ledgerKeys(t)[0]
	var mu sync.Mutex
	var paths []string
	for _, srv := range []*httptest.Server{s.api, s.portal} {
		h := srv.Config.Handler
		srv.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			paths = append(paths, r.URL.Path)
			mu.Unlock()
			h.ServeHTTP(w, r)
		})
	}
	for _, args := range [][]string{
		{"download", "fetch", "--account", "acct-2", "--dest", t.TempDir(), "--year", "2022", "--batch", key},
		{"download", "submit", "--account", "acct-2", "--batch", key, "--resume", "--yes"},
	} {
		if _, _, err := runDL(t, s, args...); err == nil || ExitCode(err) != 2 {
			t.Fatalf("%s for another account: want exit 2, got %v", args[1], err)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if len(paths) != 0 {
		t.Fatalf("a refused run reached the portal or the API: %v", paths)
	}
}
