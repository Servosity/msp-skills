package cli

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"

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
