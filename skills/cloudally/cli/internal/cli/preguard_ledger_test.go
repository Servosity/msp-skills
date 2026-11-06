// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cloudally-pp-cli/internal/cliutil/testenv"
	"cloudally-pp-cli/internal/store"
)

// A ledger written before this version's guard (by a pre-release build, which
// had no guard or did not register its batches) may record restores and
// archives that were sent. Nothing here can check that, so 'restore submit',
// 'download submit' and 'download start' refuse before any request, exit 2,
// and name the file; 'restore history', 'restore watch' and 'download fetch
// --batch' still read it.

// writePreGuardRestoreLedger writes one sent restore into this login's own
// ledger store the way a pre-release build did: the batch and its row, and no
// record of either in the restore guard.
func writePreGuardRestoreLedger(t *testing.T) string {
	t.Helper()
	configureDefaultDBScope("")
	path := defaultDBPath(restoreCLIName)
	st, err := store.OpenWithContext(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	db := st.DB()
	b := restoreBatch{BatchID: "20261001-120000-0ld1", BatchKey: "k-old", ResumeKey: "r-old", CreatedAt: time.Now().UTC(),
		Scope: scopeDistributor, TaskID: "64b0e9", Source: "MS365", Dates: []string{"2026-10-01"}, Options: map[string]any{}}
	if err := ledgerInsertBatch(db, b); err != nil {
		t.Fatal(err)
	}
	if err := ledgerInsertJob(db, restoreJob{BatchID: b.BatchID, Entity: "a@contoso.com", Requested: "2026-10-01",
		ResolvedRaw: fakeBackupRaw, Resolved: "x"}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE restore_jobs SET status = 'SUBMITTED', submit_status = 'SUBMITTED', http_status = 200 WHERE batch_id = ?`, b.BatchID); err != nil {
		t.Fatal(err)
	}
	return path
}

func wantPreGuardRefusal(t *testing.T, what string, err error, kind, path string) {
	t.Helper()
	if err == nil || ExitCode(err) != 2 || !errors.Is(err, errPreGuardLedger) {
		t.Fatalf("%s: want the pre-guard ledger refusal (exit 2), got %v", what, err)
	}
	want := "found a " + kind + " ledger written before this version's guard (" + path + "); check what it sent with `"
	if !strings.Contains(err.Error(), want) || !strings.HasSuffix(err.Error(), "then move the file away") {
		t.Fatalf("%s: the refusal must name %s and say what to do, got: %v", what, path, err)
	}
}

func TestPreGuardRestoreLedgerRefusesSubmitBeforeAnyRequest(t *testing.T) {
	for _, withGuard := range []bool{false, true} {
		name := "no-guard"
		if withGuard {
			name = "guard-without-a-record-of-it"
		}
		t.Run(name, func(t *testing.T) {
			isolateAuthState(t)
			api := newFakeRestoreAPI(t, 0)
			if withGuard {
				g, err := openRestoreGuard(context.Background())
				if err != nil {
					t.Fatal(err)
				}
				g.Close()
			}
			path := writePreGuardRestoreLedger(t)
			_, _, err := runAuthCmd(t, "", submitArgs("--date", "2026-10-01")...)
			wantPreGuardRefusal(t, "restore submit", err, "restore", path)
			if !strings.Contains(err.Error(), "`restore history`") {
				t.Fatalf("the refusal must point at restore history: %v", err)
			}
			api.mu.Lock()
			gets := len(api.getPaths)
			api.mu.Unlock()
			if api.posts.Load() != 0 || gets != 0 {
				t.Fatalf("refused run made %d POST(s) and %d GET(s); want none", api.posts.Load(), gets)
			}

			// restore history still reads the ledger.
			stdout, _, err := runAuthCmd(t, "", "restore", "history", "--json")
			if err != nil || !strings.Contains(stdout, "20261001-120000-0ld1") {
				t.Fatalf("restore history must still read the pre-guard ledger: %v\n%s", err, stdout)
			}

			// Moved away, the run proceeds.
			if err := os.Rename(path, path+".moved"); err != nil {
				t.Fatal(err)
			}
			if _, stderr, err := runAuthCmd(t, "", submitArgs("--date", "2026-10-01")...); err != nil {
				t.Fatalf("after moving the ledger away: %v (%s)", err, stderr)
			}
			if api.posts.Load() == 0 {
				t.Fatal("after moving the ledger away nothing was sent")
			}
		})
	}
}

// A ledger this version wrote, under any login of this state directory, never
// triggers the refusal: its batches are recorded in the guard before their
// first row.
func TestRestoreLedgerWrittenByThisVersionIsNeverRefused(t *testing.T) {
	isolateAuthState(t)
	api := newFakeRestoreAPI(t, 0)
	if _, stderr, err := runAuthCmd(t, "", submitArgs("--date", "2026-10-01", "--entities", "a@contoso.com")...); err != nil {
		t.Fatalf("first: %v (%s)", err, stderr)
	}
	for _, args := range [][]string{
		submitArgs("--date", "2026-10-01", "--entities", "b@contoso.com"),
		submitArgs("--date", "2026-10-01", "--entities", "a@contoso.com", "--resume"),
	} {
		if _, stderr, err := runAuthCmd(t, "", args...); errors.Is(err, errPreGuardLedger) {
			t.Fatalf("%v: a ledger this version wrote was refused: %v (%s)", args, err, stderr)
		}
	}
	if api.posts.Load() != 2 {
		t.Fatalf("posts = %d, want a@ and b@ once each", api.posts.Load())
	}
}

// A ledger store that cannot be read fails the run closed.
func TestUnreadableRestoreLedgerRefusesSubmit(t *testing.T) {
	isolateAuthState(t)
	api := newFakeRestoreAPI(t, 0)
	dir := ledgerDataDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	bad := filepath.Join(dir, "data-unreadable.db")
	if err := os.WriteFile(bad, []byte("this is not an SQLite database, padded past the header size for good measure"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, _, err := runAuthCmd(t, "", submitArgs("--date", "2026-10-01")...)
	if err == nil || ExitCode(err) != 2 || !strings.Contains(err.Error(), bad) || !strings.Contains(err.Error(), "this run sent nothing") {
		t.Fatalf("want a fail-closed exit 2 naming %s, got %v", bad, err)
	}
	if api.posts.Load() != 0 {
		t.Fatalf("posted %d with an unreadable ledger present", api.posts.Load())
	}
}

// writePreGuardDownloadLedger writes a batch ledger the way a pre-release
// build did: on disk, with no record of it in the download guard.
func writePreGuardDownloadLedger(t *testing.T, key string) string {
	t.Helper()
	writeLegacyLedger(t, key, []downloadRow{{Entity: "a@contoso.com", SubSource: "O365MAIL", SnapshotRaw: 20221231042000, Status: dlCreated, JobID: "job-old"}})
	path, err := batchPath(key)
	if err != nil {
		t.Fatal(err)
	}
	return path
}

func TestPreGuardDownloadLedgerRefusesSubmitAndStartBeforeAnyRequest(t *testing.T) {
	for _, withGuard := range []bool{false, true} {
		name := "no-guard"
		if withGuard {
			name = "guard-without-a-record-of-it"
		}
		t.Run(name, func(t *testing.T) {
			testenv.Isolate(t)
			s := newDLStub(t)
			quickReconcile(t, s)
			s.taskEntities = []string{"a@contoso.com", "b@contoso.com"}
			if withGuard {
				g, err := openDownloadGuard(context.Background())
				if err != nil {
					t.Fatal(err)
				}
				g.Close()
			}
			path := writePreGuardDownloadLedger(t, "20261015-201500-old")
			_, _, err := runDL(t, s, downloadSubmitArgs()...)
			wantPreGuardRefusal(t, "download submit", err, "download", path)
			_, _, err = runDL(t, s, downloadSubmitArgs("--batch", "20261015-201500-old", "--resume")...)
			wantPreGuardRefusal(t, "download submit --resume of it", err, "download", path)
			_, _, err = runDL(t, s, "download", "start", "--account", "acct-1", "--task", "task-1",
				"--entity", "a@contoso.com", "--date", "2022-12-31", "--yes", "--json")
			wantPreGuardRefusal(t, "download start", err, "download", path)
			if !strings.Contains(err.Error(), "`download list`") {
				t.Fatalf("the refusal must point at download list: %v", err)
			}
			s.mu.Lock()
			creates, dashboards := len(s.creates), s.dashboardCalls
			s.mu.Unlock()
			if creates != 0 || dashboards != 0 {
				t.Fatalf("refused runs made %d create(s) and %d dashboard read(s); want none", creates, dashboards)
			}

			// download fetch --batch still reads it.
			if _, stderr, err := runDL(t, s, fetchArgs(t.TempDir(), "--batch", "20261015-201500-old")...); errors.Is(err, errPreGuardLedger) {
				t.Fatalf("download fetch --batch must still read the pre-guard ledger: %v\n%s", err, stderr)
			}

			// Moved away, the run proceeds.
			if err := os.Rename(path, filepath.Join(t.TempDir(), "moved.json")); err != nil {
				t.Fatal(err)
			}
			if _, stderr, err := runDL(t, s, downloadSubmitArgs()...); err != nil {
				t.Fatalf("after moving the ledger away: %v\n%s", err, stderr)
			}
			if n := len(s.createdAccounts()); n == 0 {
				t.Fatal("after moving the ledger away nothing was created")
			}
		})
	}
}

// A batch ledger this version wrote never triggers the refusal: the batch is
// recorded in the guard before its ledger is first saved.
func TestDownloadLedgerWrittenByThisVersionIsNeverRefused(t *testing.T) {
	testenv.Isolate(t)
	s := newDLStub(t)
	quickReconcile(t, s)
	s.taskEntities = []string{"a@contoso.com"}
	if _, stderr, err := runDL(t, s, downloadSubmitArgs("--entities", "a@contoso.com")...); err != nil {
		t.Fatalf("first: %v\n%s", err, stderr)
	}
	keys := ledgerKeys(t)
	if len(keys) != 1 {
		t.Fatalf("ledgers = %v", keys)
	}
	for _, args := range [][]string{
		downloadSubmitArgs("--batch", keys[0], "--resume"),
		downloadSubmitArgs("--entities", "a@contoso.com", "--sub-sources", "O365MAIL"),
	} {
		if _, stderr, err := runDL(t, s, args...); errors.Is(err, errPreGuardLedger) {
			t.Fatalf("%v: a ledger this version wrote was refused: %v\n%s", args, err, stderr)
		}
	}
}

// saveThisVersionBatch saves a batch ledger the way this version does: the
// batch is recorded in the download guard before its ledger is first saved.
// Tests that stand in for an earlier run of this version write ledgers with it.
func saveThisVersionBatch(t *testing.T, b *downloadBatch) error {
	t.Helper()
	g, err := openDownloadGuard(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	if err := g.recordBatch(context.Background(), b.BatchKey); err != nil {
		t.Fatal(err)
	}
	return saveBatch(b)
}
