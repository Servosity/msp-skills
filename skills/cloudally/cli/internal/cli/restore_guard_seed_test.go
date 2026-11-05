// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"cloudally-pp-cli/internal/cliutil"
)

// writeOriginalLedger writes a ledger in the original schema (before
// resume_key, auth_scope, submit_status, and the identity index), the way
// the first build left it, as one store in the data directory.
func writeOriginalLedger(t *testing.T, name string, stmts ...string) string {
	t.Helper()
	dir, err := cliutil.DataDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	schema := []string{
		`CREATE TABLE restore_batches (
			batch_id TEXT PRIMARY KEY, batch_key TEXT NOT NULL, created_at TEXT NOT NULL, scope TEXT NOT NULL,
			account_id TEXT NOT NULL DEFAULT '', task_id TEXT NOT NULL, source TEXT NOT NULL DEFAULT '',
			label TEXT NOT NULL DEFAULT '', dates TEXT NOT NULL DEFAULT '[]', options TEXT NOT NULL DEFAULT '{}', last_refresh_at TEXT)`,
		`CREATE TABLE restore_jobs (
			id INTEGER PRIMARY KEY AUTOINCREMENT, batch_id TEXT NOT NULL REFERENCES restore_batches(batch_id),
			entity TEXT NOT NULL, ext_id TEXT NOT NULL DEFAULT '', requested TEXT NOT NULL, resolved_raw INTEGER NOT NULL,
			resolved TEXT NOT NULL, sub_source TEXT NOT NULL DEFAULT '', status TEXT NOT NULL, http_status INTEGER,
			response TEXT, error TEXT, task_id TEXT, progress TEXT, submitted_at TEXT, updated_at TEXT NOT NULL,
			UNIQUE(batch_id, entity, resolved_raw))`,
	}
	for _, s := range append(schema, stmts...) {
		if _, err := raw.Exec(s); err != nil {
			t.Fatalf("%s: %v", s, err)
		}
	}
	return path
}

// Item 3: one backup resolved two ways (calendar headline vs a per
// sub-source lastBackupDate, a minute apart) is one restore; two backups
// hours apart are two.
func TestRestoreGuardSameBackupWindow(t *testing.T) {
	isolateAuthState(t)
	ctx := context.Background()
	g, err := openRestoreGuard(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	claim := func(resolved int64) error {
		_, _, err := g.claim(ctx, newRestoreIdentity("64b0e9", "a@contoso.com", resolved), claimOwner{batchID: "b"}, false, false)
		return err
	}
	if err := claim(20261001020000); err != nil {
		t.Fatal(err)
	}
	var held errRestoreClaimed
	for _, near := range []int64{20261001015900, 20261001015504, 20261001022959} {
		if err := claim(near); !errors.As(err, &held) {
			t.Fatalf("%d is the same backup as 20261001020000, want a refusal, got %v", near, err)
		}
		if conflicts, err := g.conflicts(ctx, []restoreIdentity{newRestoreIdentity("64b0e9", "a@contoso.com", near)}, claimOwner{}, false, false); err != nil || len(conflicts) != 1 {
			t.Fatalf("preflight for %d: %v %v", near, conflicts, err)
		}
	}
	for _, far := range []int64{20261001140000, 20260930020000, 20261001023001} {
		if err := claim(far); err != nil {
			t.Fatalf("%d is a different backup, want it claimable, got %v", far, err)
		}
	}
}

// Item 3 end to end: use-account style last-backup resolution a minute off
// the calendar time is refused; a backup hours later is sent.
func TestRestoreGuardSameBackupResolvedTwoWays(t *testing.T) {
	isolateAuthState(t)
	api := newFakeRestoreAPI(t, 0)
	api.set(func(f *fakeRestoreAPI) { f.backupRaw = 20261001020000 })
	if _, stderr, err := runAuthCmd(t, "", submitArgs("--date", "2026-10-01")...); err != nil {
		t.Fatalf("first: %v (%s)", err, stderr)
	}
	api.set(func(f *fakeRestoreAPI) { f.backupRaw = 20261001015900 })
	_, _, err := runAuthCmd(t, "", submitArgs("--date", "2026-10-01T23:59:59Z")...)
	wantGuardRefusal(t, "same backup, other resolution", err)
	if api.posts.Load() != 5 {
		t.Fatalf("posts = %d, want 5", api.posts.Load())
	}
	api.set(func(f *fakeRestoreAPI) { f.backupRaw = 20261001140000 })
	if _, stderr, err := runAuthCmd(t, "", submitArgs("--date", "20261001235959")...); err != nil {
		t.Fatalf("a backup twelve hours later: %v (%s)", err, stderr)
	}
	if api.posts.Load() != 10 {
		t.Fatalf("posts = %d, want 10", api.posts.Load())
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
