// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Codex round-8: write-through caching respects the sync store lock, and
// symlinked / hard-linked store aliases cannot bypass it.

package cli

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"zoom-phone-pp-cli/internal/store"
)

// localStoredRows counts stored rows of a resource type and logged row
// events (0 when the store does not exist).
func localStoredRows(t *testing.T, path, resourceType string) (rows, events int) {
	t.Helper()
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return 0, 0
	}
	db, err := store.OpenReadOnly(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_ = db.DB().QueryRow(`SELECT COUNT(1) FROM resources WHERE resource_type = ?`, resourceType).Scan(&rows)
	_ = db.DB().QueryRow(`SELECT COUNT(1) FROM zoom_row_events WHERE resource_type = ?`, resourceType).Scan(&events)
	return rows, events
}

func TestLocalWriteThroughSkipsWhileSyncHoldsLock(t *testing.T) {
	localTestEnv(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.TrimPrefix(r.URL.Path, "/v2") == "/phone/numbers" {
			_, _ = w.Write([]byte(`{"phone_numbers":[{"id":"pn-live","number":"+14155550111","source":"internal","status":"available"}],"next_page_token":"","total_records":1}`))
			return
		}
		_, _ = w.Write([]byte(`{"next_page_token":""}`))
	}))
	defer srv.Close()
	t.Setenv("ZOOM_PHONE_BASE_URL", srv.URL+"/v2")
	def := defaultDBPath(localCLIName)
	// Install the event log the way a first sync would, so writes are logged.
	if _, err := localBeginSyncEventLog(t.Context(), def); err != nil {
		t.Fatal(err)
	}
	list := func() string {
		cmd := RootCmd()
		var out, errb zoomLockedBuffer
		cmd.SetOut(&out)
		cmd.SetErr(&errb)
		cmd.SetArgs([]string{"numbers", "list-account-phone", "--json"})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("numbers list-account-phone: %v\n%s", err, errb.String())
		}
		return out.String()
	}

	release := localHoldStoreLock(t, def)
	out := list()
	release()
	if !strings.Contains(out, "pn-live") {
		t.Fatalf("live result not returned while a sync held the lock: %s", out)
	}
	if rows, events := localStoredRows(t, def, "numbers"); rows != 0 || events != 0 {
		t.Fatalf("write-through cached %d rows (%d events) while a sync held the lock", rows, events)
	}

	// Without a running sync it caches as before.
	_ = list()
	if rows, _ := localStoredRows(t, def, "numbers"); rows != 1 {
		t.Fatalf("write-through did not cache with the lock free: rows=%d", rows)
	}
}

func TestLocalStoreAliasesShareOrRefuseTheLock(t *testing.T) {
	localTestEnv(t)
	localNumbersOnlyServer(t)
	dir := t.TempDir()
	real := filepath.Join(dir, "actual.db")
	s, err := store.Open(real)
	if err != nil {
		t.Fatal(err)
	}
	_ = s.Close()

	// Symlinked alias: same lock as the real file.
	alias := filepath.Join(dir, "alias.db")
	if err := os.Symlink(real, alias); err != nil {
		t.Fatal(err)
	}
	if localSyncLockPath(alias) != localSyncLockPath(real) {
		t.Fatalf("symlink alias lock %q differs from %q", localSyncLockPath(alias), localSyncLockPath(real))
	}
	release := localHoldStoreLock(t, real)
	done := make(chan struct{})
	go func() {
		localRunSync(t, "--full", "--resources", "numbers", "--db", alias, "--json")
		close(done)
	}()
	select {
	case <-done:
		release()
		t.Fatal("a sync through a symlinked alias ran while the real store was locked")
	case <-time.After(500 * time.Millisecond):
	}
	release()
	<-done

	// Hard-linked alias: refused for sync and for write-through.
	hard := filepath.Join(dir, "hard.db")
	if err := os.Link(real, hard); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{hard, real} {
		cmd := RootCmd()
		var out, errb zoomLockedBuffer
		cmd.SetOut(&out)
		cmd.SetErr(&errb)
		cmd.SetArgs([]string{"sync", "--full", "--resources", "numbers", "--db", p, "--json"})
		err := cmd.Execute()
		if err == nil || !strings.Contains(err.Error(), "hard links") {
			t.Fatalf("sync of hard-linked %s: err=%v, want a hard-link refusal", filepath.Base(p), err)
		}
	}
	if rel, ok := localTryCacheWriteLock(hard); ok {
		rel()
		t.Fatal("write-through took the lock on a hard-linked store")
	}
}
