// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credsql

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// openFiles counts this process's open descriptors; on Windows, where an
// open file cannot be removed, it reports whether path is still held. A
// scanner can hold a just-closed file for a moment, so removal gets the same
// two seconds testing.TempDir's own cleanup gives it: a leaked connection
// holds the file for the life of the process and still fails.
func openFiles(t *testing.T, path string) int {
	t.Helper()
	if runtime.GOOS == "windows" {
		for deadline := time.Now().Add(2 * time.Second); ; time.Sleep(10 * time.Millisecond) {
			err := os.Remove(path)
			if err == nil || os.IsNotExist(err) {
				return 0
			}
			if time.Now().After(deadline) {
				return 1
			}
		}
	}
	f, err := os.Open("/dev/fd")
	if err != nil {
		t.Skipf("no /dev/fd: %v", err)
	}
	defer f.Close()
	names, err := f.Readdirnames(-1)
	if err != nil {
		t.Skipf("reading /dev/fd: %v", err)
	}
	return len(names)
}

// TestTimedOutQueryClosesItsFile (handfixes.json credsql-timed-out-query-
// closes-its-file): modernc.org/sqlite before v1.61 dropped the rows a query
// had already produced when the query's context ended before it returned.
// The statement was never finalized, so closing the connection left the
// database file open for the life of the process: on Windows the file could
// no longer be deleted (TestArchiveFullResetIsNotComplete's TempDir cleanup
// failed on it). Deadlines are swept across the query so one lands in that
// window.
func TestTimedOutQueryClosesItsFile(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 200; i++ {
		path := filepath.Join(dir, "q.db")
		before := openFiles(t, path+".missing")
		db, err := sql.Open(Driver, path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
		if err != nil {
			t.Fatal(err)
		}
		c, err := db.Conn(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(i%40)*25*time.Microsecond)
		var v int
		qerr := c.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&v)
		cancel()
		_ = c.Close()
		_ = db.Close()
		if runtime.GOOS != "windows" {
			// Every descriptor the connection opened must be closed again.
			if after := openFiles(t, path); after > before {
				t.Fatalf("iteration %d: %d descriptors open after Close, %d before Open (query error: %v)", i, after, before, qerr)
			}
		} else if openFiles(t, path) != 0 {
			t.Fatalf("iteration %d: the database file is still open after Close (query error: %v)", i, qerr)
		}
		for _, sfx := range []string{"", "-wal", "-shm"} {
			_ = os.Remove(path + sfx)
		}
	}
}

// TestDeadlineInterruptsARunningQuery: the fix for the file above must not
// cost a query its interruption. The MCP sql and search tools bound user SQL
// with a context deadline, so a statement that never finishes on its own has
// to end when the deadline does.
func TestDeadlineInterruptsARunningQuery(t *testing.T) {
	db, err := sql.Open(Driver, filepath.Join(t.TempDir(), "i.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	errc := make(chan error, 1)
	go func() {
		var n int64
		errc <- db.QueryRowContext(ctx, `WITH RECURSIVE n(x) AS (VALUES(0) UNION ALL SELECT 0 FROM n) SELECT sum(x) FROM n`).Scan(&n)
	}()
	select {
	case err := <-errc:
		if err == nil {
			t.Fatal("a query that never finishes returned a result")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the deadline did not interrupt a running query")
	}
}
