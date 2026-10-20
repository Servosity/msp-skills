// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"io"
	"net/http"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"cloudally-pp-cli/internal/cliutil/testenv"
	"cloudally-pp-cli/internal/portal"
	"github.com/spf13/cobra"
)

// newSubmitBatch saves a ledger of PLANNED rows, one per mailbox.
func newSubmitBatch(t *testing.T, key string, entities ...string) *downloadBatch {
	t.Helper()
	b := &downloadBatch{
		BatchKey: key, Scope: restoreScope{account: "acct-1"}.kind(), AccountID: "acct-1",
		TaskID: "task-1", Source: "MS365", OnDate: "2022-12-31", Timezone: "UTC", Format: downloadFormatZip,
		IncludeDeleted: new(bool), IncludeLegalHold: new(bool),
	}
	for i, e := range entities {
		b.Rows = append(b.Rows, downloadRow{
			Entity: e, ExtID: "ext-" + e, SubSource: "O365MAIL",
			SnapshotRaw: 20221231042000 + int64(i), Status: dlPlanned,
		})
	}
	if err := saveBatch(b); err != nil {
		t.Fatal(err)
	}
	return b
}

func openStubSession(t *testing.T, s *dlStub) *portal.Session {
	t.Helper()
	sess, err := portal.Open(context.Background(), portal.Config{BaseURL: s.portal.URL, RatePerSec: -1}, "tok")
	if err != nil {
		t.Fatalf("open portal session: %v", err)
	}
	return sess
}

func quietCmd() *cobra.Command {
	c := &cobra.Command{}
	c.SetErr(io.Discard)
	c.SetOut(io.Discard)
	return c
}

func rowByEntity(b *downloadBatch, entity string) downloadRow {
	for _, r := range b.Rows {
		if r.Entity == entity {
			return r
		}
	}
	return downloadRow{}
}

// TestRunDownloadBatchSavesRequestedBeforeEachCreate reads the ledger from
// disk at the moment each create reaches the portal: the row must already
// say REQUESTED there, or a kill -9 at that instant loses it.
func TestRunDownloadBatchSavesRequestedBeforeEachCreate(t *testing.T) {
	testenv.Isolate(t)
	s := newDLStub(t)
	b := newSubmitBatch(t, "20221231-000000-persist", "a@contoso.com", "b@contoso.com", "c@contoso.com")
	var mu sync.Mutex
	seen := map[string]string{}
	s.onCreate = func(body map[string]any) {
		onDisk, err := loadBatch("20221231-000000-persist")
		if err != nil {
			t.Errorf("reading the ledger mid-run: %v", err)
			return
		}
		account, _ := body["account"].(string)
		r := rowByEntity(onDisk, account)
		mu.Lock()
		seen[account] = r.Status
		if r.RequestedAt == "" {
			seen[account] += " without requested_at"
		}
		mu.Unlock()
	}
	sess := openStubSession(t, s)
	if err := runDownloadBatch(context.Background(), quietCmd(), sess, b, pendingDownloadRows(b, false, false), downloadFormatZip, 2, false, false); err != nil {
		t.Fatalf("runDownloadBatch: %v", err)
	}
	if len(seen) != 3 {
		t.Fatalf("want 3 creates, saw %v", seen)
	}
	for account, status := range seen {
		if status != dlRequested {
			t.Fatalf("%s was %q on disk when its create arrived, want REQUESTED", account, status)
		}
	}
	after, err := loadBatch("20221231-000000-persist")
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range after.Rows {
		if r.Status != dlRequested || r.AcceptedAt == "" {
			t.Fatalf("after a clean run every row is REQUESTED with accepted_at: %+v", r)
		}
	}
}

// TestRunDownloadBatchClassifiesAmbiguousFailuresAsUnknown covers the
// three outcomes of a failed create: a server error after the request left
// is UNKNOWN, a refused session is FAILED, and a request that never reached
// the portal is FAILED.
func TestRunDownloadBatchClassifiesAmbiguousFailuresAsUnknown(t *testing.T) {
	testenv.Isolate(t)
	s := newDLStub(t)
	s.createCode["a@contoso.com"] = http.StatusInternalServerError
	s.createCode["b@contoso.com"] = http.StatusUnauthorized
	b := newSubmitBatch(t, "20221231-000000-classify", "a@contoso.com", "b@contoso.com", "c@contoso.com")
	sess := openStubSession(t, s)
	if err := runDownloadBatch(context.Background(), quietCmd(), sess, b, pendingDownloadRows(b, false, false), downloadFormatZip, 1, false, false); err != nil {
		t.Fatalf("runDownloadBatch: %v", err)
	}
	onDisk, err := loadBatch("20221231-000000-classify")
	if err != nil {
		t.Fatal(err)
	}
	if r := rowByEntity(onDisk, "a@contoso.com"); r.Status != dlUnknown {
		t.Fatalf("a 500 after the request left must be UNKNOWN, got %+v", r)
	}
	if r := rowByEntity(onDisk, "b@contoso.com"); r.Status != dlFailed {
		t.Fatalf("a refused session must be FAILED, got %+v", r)
	}
	if r := rowByEntity(onDisk, "c@contoso.com"); r.Status != dlRequested || r.AcceptedAt == "" {
		t.Fatalf("an accepted create must be REQUESTED with accepted_at, got %+v", r)
	}

	// A portal that is gone before the request is sent: nothing left.
	gone := newDLStub(t)
	b2 := newSubmitBatch(t, "20221231-000000-gone", "d@contoso.com")
	sess2 := openStubSession(t, gone)
	gone.portal.Close()
	if err := runDownloadBatch(context.Background(), quietCmd(), sess2, b2, pendingDownloadRows(b2, false, false), downloadFormatZip, 1, false, false); err != nil {
		t.Fatalf("runDownloadBatch: %v", err)
	}
	if r := rowByEntity(b2, "d@contoso.com"); r.Status != dlFailed {
		t.Fatalf("a create that never connected must be FAILED, got %+v", r)
	}
}

// TestDownloadSubmitResumeAfterCrashSendsNothingTwice simulates a kill -9
// at the first create by restoring the ledger exactly as it stood on disk
// at that instant, then resumes. The interrupted row may exist, so it is
// never sent again on its own; once the portal lists it, it is CREATED and
// not sent even under --retry-unknown; an UNKNOWN row the portal does not
// list is sent only with --retry-unknown.
func TestDownloadSubmitResumeAfterCrashSendsNothingTwice(t *testing.T) {
	testenv.Isolate(t)
	prevDelay := reconcileDelay
	reconcileDelay = time.Millisecond
	t.Cleanup(func() { reconcileDelay = prevDelay })

	s := newDLStub(t)
	b := newSubmitBatch(t, "20221231-000000-crash", "a@contoso.com", "b@contoso.com", "c@contoso.com")
	path, err := batchPath("20221231-000000-crash")
	if err != nil {
		t.Fatal(err)
	}
	var crashState []byte
	var crashed string
	s.onCreate = func(body map[string]any) {
		if crashState == nil {
			crashState, _ = os.ReadFile(path)
			crashed, _ = body["account"].(string)
		}
	}
	sess := openStubSession(t, s)
	if err := runDownloadBatch(context.Background(), quietCmd(), sess, b, pendingDownloadRows(b, false, false), downloadFormatZip, 1, false, false); err != nil {
		t.Fatalf("runDownloadBatch: %v", err)
	}
	if err := os.WriteFile(path, crashState, batchFilePerm); err != nil {
		t.Fatal(err)
	}
	// The create in flight at the crash, one that answers 500 on resume,
	// and one that goes through.
	var others []string
	for _, e := range []string{"a@contoso.com", "b@contoso.com", "c@contoso.com"} {
		if e != crashed {
			others = append(others, e)
		}
	}
	flaky, clean := others[0], others[1]
	onDisk, _ := loadBatch("20221231-000000-crash")
	if r := rowByEntity(onDisk, crashed); r.Status != dlRequested || r.AcceptedAt != "" {
		t.Fatalf("the crash state should hold %s REQUESTED and unanswered: %+v", crashed, r)
	}
	for _, e := range others {
		if r := rowByEntity(onDisk, e); r.Status != dlPlanned {
			t.Fatalf("the crash state should hold %s PLANNED: %+v", e, r)
		}
	}

	// Resume. The interrupted row is held; the flaky one answers 500.
	s.mu.Lock()
	s.creates, s.onCreate = nil, nil
	s.createCode[flaky] = http.StatusInternalServerError
	s.mu.Unlock()
	_, stderr, err := runDL(t, s, "download", "submit", "--batch", "20221231-000000-crash", "--resume", "--yes", "--json")
	if err == nil || ExitCode(err) != 6 {
		t.Fatalf("a batch left with UNKNOWN rows must exit 6, got %v\n%s", err, stderr)
	}
	if got := s.createdAccounts(); slices.Contains(got, crashed) {
		t.Fatalf("the interrupted create for %s was sent again: %v", crashed, got)
	}
	if got := sortedCopy(s.createdAccounts()); !slices.Equal(got, sortedCopy(others)) {
		t.Fatalf("creates = %v, want %v", got, others)
	}
	if !strings.Contains(stderr, "UNKNOWN") || !strings.Contains(stderr, "--retry-unknown") {
		t.Fatalf("the operator should be told how to settle UNKNOWN rows:\n%s", stderr)
	}
	after, _ := loadBatch("20221231-000000-crash")
	if r := rowByEntity(after, crashed); r.Status != dlUnknown {
		t.Fatalf("%s should now be UNKNOWN: %+v", crashed, r)
	}
	if r := rowByEntity(after, flaky); r.Status != dlUnknown {
		t.Fatalf("%s should be UNKNOWN after a 500: %+v", flaky, r)
	}

	// The portal now lists the interrupted row (the crashed run did create
	// it) and the clean one, which reconcile had marked MISSING.
	s.mu.Lock()
	s.creates = nil
	delete(s.createCode, flaky)
	for _, e := range []string{crashed, clean} {
		r := rowByEntity(after, e)
		s.jobs = append(s.jobs, dlJob{JobID: "job-" + e, Account: e, SubSource: r.SubSource, Status: "INPROCESS", Snapshot: r.SnapshotRaw})
	}
	s.mu.Unlock()
	_, stderr, err = runDL(t, s, "download", "submit", "--batch", "20221231-000000-crash", "--resume", "--retry-unknown", "--yes", "--json")
	// The re-sent row is not on the portal yet, so it ends MISSING and the
	// batch is reported as not settled.
	if err == nil || ExitCode(err) != 6 || !strings.Contains(err.Error(), "1 MISSING") {
		t.Fatalf("resume --retry-unknown: want exit 6 with 1 MISSING, got %v\n%s", err, stderr)
	}
	if got := s.createdAccounts(); !slices.Equal(got, []string{flaky}) {
		t.Fatalf("only %s (UNKNOWN and not on the portal) may be sent, got %v", flaky, got)
	}
	final, _ := loadBatch("20221231-000000-crash")
	for _, e := range []string{crashed, clean} {
		if r := rowByEntity(final, e); r.Status != dlCreated || r.JobID != "job-"+e {
			t.Fatalf("%s should be CREATED from the portal listing: %+v", e, r)
		}
	}
}

func TestDownloadSubmitRetryUnknownNeedsResume(t *testing.T) {
	testenv.Isolate(t)
	s := newDLStub(t)
	_, _, err := runDL(t, s, "download", "submit", "--account", "acct-1", "--task", "task-1", "--on-date", "2022-12-31", "--retry-unknown", "--yes")
	if err == nil || !strings.Contains(err.Error(), "--retry-unknown") || ExitCode(err) != 2 {
		t.Fatalf("want a usage error, got %v", err)
	}
}

func sortedCopy(in []string) []string {
	out := append([]string(nil), in...)
	slices.Sort(out)
	return out
}
