// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package store

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// migrationStatements are the CREATE ... IF NOT EXISTS statements written in
// this package's store.go, where migrate's list lives.
func migrationStatements(t *testing.T) []string {
	t.Helper()
	src, err := os.ReadFile("store.go")
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, m := range regexp.MustCompile("`(CREATE [^`]*IF NOT EXISTS[^`]*)`").FindAllStringSubmatch(string(src), -1) {
		out = append(out, m[1])
	}
	if len(out) < 400 {
		t.Fatalf("found %d CREATE statements in store.go; migrate's list has about 480", len(out))
	}
	return out
}

// Hand-fix store-open-runs-only-missing-objects: every statement of migrate's
// list names its object, so an open of a current store runs none of them; a
// store with nothing in it runs every one.
func TestReopenRunsNoMigrationStatement(t *testing.T) {
	ctx := context.Background()
	stmts := migrationStatements(t)
	for _, m := range stmts {
		if _, ok := createdObject(m); !ok {
			t.Errorf("migrate statement names no object, so every open runs it:\n%.200s", m)
		}
	}
	path := filepath.Join(t.TempDir(), "data.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	conn, err := s.db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	pending, err := pendingMigrations(ctx, conn, stmts)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 0 {
		t.Errorf("a current store still runs %d migrate statements, first:\n%.200s", len(pending), pending[0])
	}

	empty, err := Open(filepath.Join(t.TempDir(), "other.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer empty.Close()
	ec, err := empty.db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer ec.Close()
	if _, err := ec.ExecContext(ctx, `DROP TABLE sync_state`); err != nil {
		t.Fatal(err)
	}
	pending, err = pendingMigrations(ctx, ec, stmts)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 || pending[0] != stmtCreating(t, stmts, "sync_state") {
		t.Errorf("a dropped table: want only its CREATE to run, got %d statements", len(pending))
	}
}

func stmtCreating(t *testing.T, stmts []string, table string) string {
	t.Helper()
	for _, m := range stmts {
		if k, _ := createdObject(m); k == "table\x00"+table {
			return m
		}
	}
	t.Fatalf("no statement creates %s", table)
	return ""
}

// The name is taken only from the shapes migrate writes; anything else runs.
func TestCreatedObjectNamesOnlyWhatItCanRead(t *testing.T) {
	for stmt, want := range map[string]string{
		"CREATE TABLE IF NOT EXISTS resources (\n id TEXT)":                     "table\x00resources",
		`CREATE INDEX IF NOT EXISTS "idx_A" ON "t"("c")`:                        "index\x00idx_a",
		"CREATE UNIQUE INDEX IF NOT EXISTS u1 ON t(c)":                          "index\x00u1",
		"CREATE VIRTUAL TABLE IF NOT EXISTS resources_fts USING fts5(x)":        "table\x00resources_fts",
		"\n\t\tCREATE TRIGGER IF NOT EXISTS trg_x AFTER INSERT ON t BEGIN END;": "trigger\x00trg_x",
	} {
		if got, ok := createdObject(stmt); !ok || got != want {
			t.Errorf("%q: got %q %v, want %q", stmt, got, ok, want)
		}
	}
	for _, stmt := range []string{
		"CREATE TABLE resources (id TEXT)",
		`CREATE TABLE IF NOT EXISTS "a"."b" (id TEXT)`,
		`CREATE TABLE IF NOT EXISTS main.b (id TEXT)`,
		`CREATE TABLE IF NOT EXISTS "a""b" (id TEXT)`,
		"CREATE VIEW IF NOT EXISTS v AS SELECT 1",
		"ALTER TABLE t ADD COLUMN c",
	} {
		if got, ok := createdObject(stmt); ok {
			t.Errorf("%q: named %q; a statement it cannot read must run", stmt, got)
		}
	}
}

// execMigrations runs every statement, a trailing -- comment included, and
// stops at the first that fails.
func TestExecMigrationsRunsEachAndStopsAtAFailure(t *testing.T) {
	ctx := context.Background()
	s, err := Open(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	conn, err := s.db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	has := func(name string) bool {
		var n int
		if err := conn.QueryRowContext(ctx, `SELECT count(*) FROM sqlite_master WHERE name = ?`, name).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n == 1
	}
	if err := execMigrations(ctx, conn, []string{
		"CREATE TABLE IF NOT EXISTS mig_a (x TEXT) -- a trailing comment",
		"CREATE TABLE IF NOT EXISTS mig_b (x TEXT)",
	}); err != nil || !has("mig_a") || !has("mig_b") {
		t.Fatalf("want both tables: err %v, a %v, b %v", err, has("mig_a"), has("mig_b"))
	}
	if err := execMigrations(ctx, conn, []string{
		"CREATE TABLE mig_a (x TEXT)",
		"CREATE TABLE IF NOT EXISTS mig_c (x TEXT)",
	}); err == nil || has("mig_c") {
		t.Fatalf("a failing statement: want its error and nothing after it, got err %v, mig_c %v", err, has("mig_c"))
	}
}
