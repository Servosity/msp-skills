// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Codex round-7: first sync of a fresh account, every syncResource entry
// point under one store lock, unattributed writes invalidate walks, and a
// walk is published in one transaction.

package cli

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"zoom-phone-pp-cli/internal/cliutil"
	"zoom-phone-pp-cli/internal/store"
)

func localNumbersOnlyServer(t *testing.T) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.TrimPrefix(r.URL.Path, "/v2") == "/phone/numbers" {
			_, _ = w.Write([]byte(`{"phone_numbers":[` + localTestNumbers[0] + `],"next_page_token":""}`))
			return
		}
		_, _ = w.Write([]byte(`{"next_page_token":""}`))
	}))
	t.Cleanup(srv.Close)
	t.Setenv("ZOOM_PHONE_BASE_URL", srv.URL+"/v2")
}

// localHoldStoreLock holds the canonical sync lock until release is closed.
func localHoldStoreLock(t *testing.T, dbPath string) (release func()) {
	t.Helper()
	held, rel, freed := make(chan struct{}), make(chan struct{}), make(chan struct{})
	go func() {
		_ = cliutil.WithFileLock(localSyncLockPath(dbPath), func() error {
			close(held)
			<-rel
			return nil
		})
		close(freed)
	}()
	<-held
	return func() { close(rel); <-freed }
}

// ---- item 1: a brand-new account's first sync ----

func TestLocalFirstSyncCreatesPrivateStoreDir(t *testing.T) {
	localTestEnv(t)
	localNumbersOnlyServer(t)
	def := defaultDBPath(localCLIName)
	dir := filepath.Dir(def)
	if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("fresh HOME already has %s (err=%v)", dir, err)
	}
	cmd := RootCmd()
	var out, errb zoomLockedBuffer
	cmd.SetOut(&out)
	cmd.SetErr(&errb)
	cmd.SetArgs([]string{"sync", "--full", "--resources", "numbers", "--json"})
	err := cmd.Execute()
	if err != nil || strings.Contains(errb.String(), "could not lock") {
		t.Fatalf("first sync of a fresh account failed: err=%v stderr=%s", err, errb.String())
	}
	st, statErr := os.Stat(dir)
	if statErr != nil {
		t.Fatalf("store dir %s: err=%v", dir, statErr)
	}
	if runtime.GOOS != "windows" && st.Mode().Perm() != 0o700 {
		t.Fatalf("store dir %s: mode=%v, want 0700", dir, st.Mode().Perm())
	}
	if scope := localScopeRows(t, def); scope["numbers"] != "0|1|" {
		t.Fatalf("first sync scope = %v", scope)
	}
}

// ---- item 2: the other syncResource entry points take the store lock ----

func TestLocalAutoRefreshSkipsWhileSyncHoldsLock(t *testing.T) {
	localTestEnv(t)
	localNumbersOnlyServer(t)
	path := localTestStore(t, map[string][]string{"numbers": localTestNumbers})
	db, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	release := localHoldStoreLock(t, path)
	err = runAutoRefresh(context.Background(), &rootFlags{}, db, []string{"numbers"})
	release()
	if err == nil || !strings.Contains(err.Error(), errLocalSyncBusy.Error()) {
		t.Fatalf("auto-refresh while a sync holds the lock: err=%v, want %q", err, errLocalSyncBusy)
	}
	// With the lock free it refreshes normally.
	if err := runAutoRefresh(context.Background(), &rootFlags{}, db, []string{"numbers"}); err != nil {
		t.Fatalf("auto-refresh with the lock free: %v", err)
	}
}

func TestLocalWorkflowArchiveWaitsForStoreLock(t *testing.T) {
	localTestEnv(t)
	localNumbersOnlyServer(t)
	path := filepath.Join(t.TempDir(), "data.db")
	release := localHoldStoreLock(t, path)
	done := make(chan string, 1)
	go func() {
		cmd := RootCmd()
		var out, errb zoomLockedBuffer
		cmd.SetOut(&out)
		cmd.SetErr(&errb)
		cmd.SetArgs([]string{"workflow", "archive", "--db", path, "--json"})
		_ = cmd.Execute()
		done <- errb.String()
	}()
	select {
	case <-done:
		release()
		t.Fatal("workflow archive ran while a sync held the store lock")
	case <-time.After(500 * time.Millisecond):
	}
	release()
	select {
	case stderr := <-done:
		if !strings.Contains(stderr, "another sync is running against") {
			t.Fatalf("archive did not report waiting: %q", stderr)
		}
	case <-time.After(60 * time.Second):
		t.Fatal("archive did not finish after the lock was released")
	}
}

// A write the recorder cannot attribute (here a direct store write, as the
// auto-refresh, archive or write-through caching make) rewrites a BYOC walk
// row: the walk is treated as superseded at once, and stays superseded
// after the next recorded sync absorbs the event.
func TestLocalUnattributedWriteInvalidatesWalk(t *testing.T) {
	localTestEnv(t)
	localNewByocPagedServer(t)
	db := filepath.Join(t.TempDir(), "data.db")
	localRunSync(t, "--full", "--resources", "users,numbers", "--db", db, "--json")
	localRunSync(t, "--resources", "numbers", "--db", db, "--resource-param", "numbers:type=byoc", "--json")
	if got := localByocFindings(localE911(t, db)); got != "pn-2,pn-3" {
		t.Fatalf("setup BYOC = %q", got)
	}

	s, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.UpsertBatch("numbers", []json.RawMessage{json.RawMessage(`{"id":"pn-2","number":"+15125550102","source":"external","status":"available"}`)}); err != nil {
		t.Fatal(err)
	}
	_ = s.Close()

	e := localE911(t, db)
	if localByocFindings(e) != "" || !strings.Contains(strings.Join(e.SkippedChecks, "\n"), "replaced the BYOC walk's rows") {
		t.Fatalf("after an unattributed write: BYOC=%q skipped=%v", localByocFindings(e), e.SkippedChecks)
	}
	// An unrelated recorded sync absorbs the event: still superseded.
	localRunSync(t, "--full", "--resources", "users", "--db", db, "--json")
	e = localE911(t, db)
	if localByocFindings(e) != "" || !strings.Contains(strings.Join(e.SkippedChecks, "\n"), "replaced the BYOC walk's rows") {
		t.Fatalf("after the next sync: BYOC=%q skipped=%v", localByocFindings(e), e.SkippedChecks)
	}
}

// ---- item 3: a walk is published in one transaction ----

func TestLocalWalkPublishIsAtomic(t *testing.T) {
	localTestEnv(t)
	localNewByocPagedServer(t)
	db := filepath.Join(t.TempDir(), "data.db")
	byocArgs := []string{"--resources", "numbers", "--db", db, "--resource-param", "numbers:type=byoc", "--json"}
	localRunSync(t, "--full", "--resources", "users,numbers", "--db", db, "--json")
	localRunSync(t, byocArgs...)

	localWalkRowInsertHook = func(id string) error { return errors.New("injected insert failure") }
	t.Cleanup(func() { localWalkRowInsertHook = nil })
	cmd := RootCmd()
	var out, errb zoomLockedBuffer
	cmd.SetOut(&out)
	cmd.SetErr(&errb)
	cmd.SetArgs(append([]string{"sync"}, byocArgs...))
	_ = cmd.Execute()
	localWalkRowInsertHook = nil
	if !strings.Contains(errb.String(), "stay pending") {
		t.Fatalf("recording failure not reported as pending: %s", errb.String())
	}

	ro, err := store.OpenReadOnly(db)
	if err != nil {
		t.Fatal(err)
	}
	w := localReadWalks(ro.DB(), "numbers")["type=byoc"]
	_ = ro.Close()
	if !w.pending || !w.rows["pn-2"] || !w.rows["pn-3"] || len(w.rows) != 2 {
		t.Fatalf("after a failed publish: pending=%v rows=%v, want pending with the old rows pn-2,pn-3", w.pending, w.rows)
	}
	if e := localE911(t, db); localByocFindings(e) != "" || !strings.Contains(strings.Join(e.SkippedChecks, "\n"), "did not complete") {
		t.Fatalf("after a failed publish: BYOC=%q skipped=%v", localByocFindings(e), e.SkippedChecks)
	}
}

// workflow archive --full resets checkpoints only once it holds the store
// lock, so a concurrent sync never sees the reset mid-walk.
func TestLocalArchiveFullResetWaitsForStoreLock(t *testing.T) {
	localTestEnv(t)
	localNumbersOnlyServer(t)
	path := filepath.Join(t.TempDir(), "data.db")
	s, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SaveSyncState("alert_settings", "cursor-x", 7); err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
	count := func() int {
		ro, err := store.OpenReadOnly(path)
		if err != nil {
			t.Fatal(err)
		}
		defer ro.Close()
		var n int
		if err := ro.DB().QueryRow(`SELECT total_count FROM sync_state WHERE resource_type = 'alert_settings'`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}

	release := localHoldStoreLock(t, path)
	done := make(chan string, 1)
	go func() {
		cmd := RootCmd()
		var out, errb zoomLockedBuffer
		cmd.SetOut(&out)
		cmd.SetErr(&errb)
		cmd.SetArgs([]string{"workflow", "archive", "--full", "--db", path, "--json"})
		_ = cmd.Execute()
		done <- errb.String()
	}()
	select {
	case <-done:
		release()
		t.Fatal("workflow archive --full ran while a sync held the store lock")
	case <-time.After(500 * time.Millisecond):
	}
	if n := count(); n != 7 {
		release()
		<-done
		t.Fatalf("archive --full reset the checkpoint (total_count %d) while the lock was held", n)
	}
	release()
	select {
	case <-done:
	case <-time.After(60 * time.Second):
		t.Fatal("archive did not finish after the lock was released")
	}
	if n := count(); n == 7 {
		t.Fatalf("archive --full never reset the checkpoint after the lock was released")
	}
}
