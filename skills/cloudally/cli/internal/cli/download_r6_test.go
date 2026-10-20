package cli

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"cloudally-pp-cli/internal/cliutil/testenv"
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
			for effect := dropped; effect <= late; effect++ {
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
		if _, err := g.db.Exec(`UPDATE download_claims SET job_id = ?, updated_at = ?`, jobID, isoUTC(updated)); err != nil {
			t.Fatal(err)
		}
	}
	long := since.Add(-time.Hour)
	for _, tc := range []struct {
		name               string
		state, job, owner  string
		updated            time.Time
		unsettled, changed bool
	}{
		{"unknown long ago", dgUnknown, "", "other", long, true, false},
		{"in flight", dgInFlight, "", "other", long, true, false},
		{"accepted long ago", dgCreated, "", "other", long, false, false},
		{"accepted just before", dgCreated, "", "other", since.Add(-time.Second), true, false},
		{"settled in the window", dgCreated, "", "other", since.Add(2 * time.Second), true, true},
		{"refused in the window", dgRejected, "", "other", since.Add(2 * time.Second), false, true},
		{"confirmed in the window", dgCreated, "job-1", "other", since.Add(2 * time.Second), false, false},
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
	if !strings.Contains(out, "would resolve the backup on 2022-12-31") || !strings.Contains(out, "EXPORT job per mailbox and sub-source") {
		t.Fatalf("--dry-run should say what it would do:\n%s", out)
	}
	if len(ledgerKeys(t)) != 0 {
		t.Fatal("--dry-run saved a ledger")
	}
}
