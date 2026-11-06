// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated; handfixes.json credkit-sqlite-every-handle):
// the value-length cap the MCP sql tool relies on is set on each connection
// as it opens under the kit's driver, and a cap that cannot be read refuses
// the open rather than running uncapped.

package credsql

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

func TestLengthLimitAppliesToEveryConnection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cap.db")
	db, err := sql.Open(Driver, "file:"+path+"?"+LengthLimitParam+"=1000")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(3)
	for i := 0; i < 3; i++ {
		var n int
		if err := db.QueryRow("SELECT length(zeroblob(1000))").Scan(&n); err != nil || n != 1000 {
			t.Fatalf("a value at the cap failed: n=%d err=%v", n, err)
		}
		var b []byte
		err := db.QueryRow("SELECT zeroblob(1001)").Scan(&b)
		var se *sqlite.Error
		if !errors.As(err, &se) || se.Code() != sqlite3.SQLITE_TOOBIG {
			t.Fatalf("a value over the cap was not SQLITE_TOOBIG: %v (%d bytes)", err, len(b))
		}
	}
}

func TestBadLengthLimitRefusesTheOpen(t *testing.T) {
	for _, q := range []string{"0", "-1", "x", "1000000001", "10&" + LengthLimitParam + "=20"} {
		db, err := sql.Open(Driver, "file:"+filepath.Join(t.TempDir(), "cap.db")+"?"+LengthLimitParam+"="+q)
		if err != nil {
			t.Fatal(err)
		}
		if err := db.Ping(); err == nil {
			t.Errorf("%s=%s opened a connection", LengthLimitParam, q)
		}
		_ = db.Close()
	}
	db, err := sql.Open(Driver, "file:"+filepath.Join(t.TempDir(), "plain.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		t.Fatalf("a DSN without %s did not open: %v", LengthLimitParam, err)
	}
}
