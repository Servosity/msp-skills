// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): a store an older build wrote is scrubbed of
// every credential it holds, in every table, exactly once, and a scrub that
// fails leaves the store below the schema version that marks it scrubbed.

package store

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
)

// codexRound2Passphrase is the value of the Codex round 2 failing input: a
// typed actions row an older build stored with the agent passphrase in it.
const codexRound2Passphrase = "ZEBRA-QUARTZ-123"

type olderBuildStmt struct {
	q    string
	args []any
}

// writeOlderBuildStore leaves dbPath exactly as an older (schema v13) build
// left it: the current tables, the rows stmts write with no write-path
// scrubbing, user_version 13 and the v13 build's own scrub stamp.
func writeOlderBuildStore(t *testing.T, dbPath string, stmts ...olderBuildStmt) {
	t.Helper()
	s, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, st := range stmts {
		if _, err := s.DB().Exec(st.q, st.args...); err != nil {
			t.Fatalf("%s: %v", st.q, err)
		}
	}
	for _, q := range []string{
		`CREATE TABLE IF NOT EXISTS secret_scrub_state (id INTEGER PRIMARY KEY CHECK (id = 1), rule_version INTEGER NOT NULL)`,
		`INSERT INTO secret_scrub_state (id, rule_version) VALUES (1, 1) ON CONFLICT(id) DO UPDATE SET rule_version = 1`,
		`PRAGMA user_version = 13`,
	} {
		if _, err := s.DB().Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
}

func openRawStore(t *testing.T, dbPath string) *sql.DB {
	t.Helper()
	raw, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	raw.SetMaxOpenConns(1)
	return raw
}

func rawUserVersion(t *testing.T, raw *sql.DB) int {
	t.Helper()
	var v int
	if err := raw.QueryRow(`PRAGMA user_version`).Scan(&v); err != nil {
		t.Fatal(err)
	}
	return v
}

func rawCount(t *testing.T, db *sql.DB, q string, args ...any) int {
	t.Helper()
	var n int
	if err := db.QueryRow(q, args...).Scan(&n); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
	return n
}

func rawString(t *testing.T, db *sql.DB, q string, args ...any) string {
	t.Helper()
	var v sql.NullString
	if err := db.QueryRow(q, args...).Scan(&v); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
	return v.String
}

// TestOldStoreTypedTableCredentialIsScrubbed is the Codex round 2 failing
// input and its neighbors: a credential an older build left in a typed table
// the hand-picked scrub list never named (actions), in a nested resources
// field, in a stale FTS entry, in a never-stored partition (any spelling of
// its name, and a typed table of its own), in secret-named columns, in a
// learn table, and in a WITHOUT ROWID table. After the new build opens the
// store, none of it is anywhere in the file, every ordinary value is, and
// the store is at v14.
func TestOldStoreTypedTableCredentialIsScrubbed(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "data.db")
	writeOlderBuildStore(t, dbPath,
		olderBuildStmt{`INSERT INTO actions (id, agents_id, data) VALUES ('a1', 'a1', ?)`,
			[]any{`{"id":"a1","agents_id":"a1","passphrase":"` + codexRound2Passphrase + `"}`}},
		olderBuildStmt{`INSERT INTO actions (id, agents_id, data) VALUES ('a2', 'a1', ?)`,
			[]any{`{"id":"a2","agents_id":"a1","description":"ORDINARYACTION","nested":{"clientSecret":"NESTEDACTIONSECRET77"}}`}},
		olderBuildStmt{`INSERT INTO resources (id, resource_type, data) VALUES ('s1', 'sites', ?)`,
			[]any{`{"id":"s1","name":"ORDINARYSITE","settings":{"registrationToken":"SITEREGTOKEN88"}}`}},
		olderBuildStmt{`INSERT INTO resources_fts (rowid, id, resource_type, content) VALUES (?, 's1', 'sites', 'ORDINARYSITE SITEREGTOKEN88')`,
			[]any{ftsRowID("sites", "s1")}},
		olderBuildStmt{`INSERT INTO resources_fts (rowid, id, resource_type, content) VALUES (?, 'a1', 'actions', ?)`,
			[]any{ftsRowID("actions", "a1"), "a1 " + codexRound2Passphrase}},
		olderBuildStmt{`INSERT INTO resources (id, resource_type, data) VALUES ('11', 'agents-passphrases', ?)`,
			[]any{`{"id":"11","computerName":"WSPASS","passphrase":"PARTITIONPASS99"}`}},
		olderBuildStmt{`INSERT INTO resources (id, resource_type, data) VALUES ('12', 'AGENTS_PASSPHRASES', ?)`,
			[]any{`{"id":"12","value":"UPPERPARTITION55"}`}},
		olderBuildStmt{`INSERT INTO sync_state (resource_type, last_synced_at, total_count) VALUES ('agents-passphrases', '2026-01-01T00:00:00Z', 1)`, nil},
		olderBuildStmt{`CREATE TABLE agents_passphrases (id TEXT PRIMARY KEY, data JSON NOT NULL)`, nil},
		olderBuildStmt{`INSERT INTO agents_passphrases (id, data) VALUES ('11', '{"id":"11","value":"TYPEDPARTITION66"}')`, nil},
		olderBuildStmt{`ALTER TABLE uploads ADD COLUMN registration_token TEXT`, nil},
		olderBuildStmt{`ALTER TABLE uploads ADD COLUMN api_key TEXT NOT NULL DEFAULT ''`, nil},
		olderBuildStmt{`INSERT INTO uploads (id, agents_id, data, registration_token, api_key) VALUES ('u1', 'a1', ?, 'COLUMNREGTOKEN44', 'COLUMNAPIKEY33')`,
			[]any{`{"id":"u1","fileName":"ORDINARYUPLOAD"}`}},
		olderBuildStmt{`INSERT INTO learn_candidates (class, payload, derivation_signature, created_at, updated_at, last_seen_at) VALUES ('flag_alias', ?, 'sig1', 't', 't', 't')`,
			[]any{`{"flag":"--site","example":"ORDINARYLEARN","apiKey":"LEARNAPIKEY22"}`}},
		olderBuildStmt{`CREATE TABLE legacy_kv (k TEXT PRIMARY KEY, v TEXT) WITHOUT ROWID`, nil},
		olderBuildStmt{`INSERT INTO legacy_kv (k, v) VALUES ('k1', ?)`,
			[]any{`{"keep":"ORDINARYKV","password":"WRPASSWORD11"}`}},
	)
	sentinels := []string{codexRound2Passphrase, "NESTEDACTIONSECRET77", "SITEREGTOKEN88", "PARTITIONPASS99",
		"UPPERPARTITION55", "TYPEDPARTITION66", "COLUMNREGTOKEN44", "COLUMNAPIKEY33", "LEARNAPIKEY22", "WRPASSWORD11"}
	for _, sentinel := range sentinels {
		if len(secretStoreFileLeaks(t, dir, sentinel)) == 0 {
			t.Fatalf("the older-build store does not hold %q before the open", sentinel)
		}
	}

	s, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	db := s.DB()
	if v, err := s.SchemaVersion(); err != nil || v != 14 {
		t.Errorf("schema version after the open = %d (%v), want 14", v, err)
	}

	// The Codex query through MCP sql returns NULL for every actions row.
	rows, err := db.Query(`SELECT json_extract(data,'$.passphrase') AS v FROM actions`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var v sql.NullString
		if err := rows.Scan(&v); err != nil {
			t.Fatal(err)
		}
		if v.Valid {
			t.Errorf("Codex query returned %q, want NULL", v.String)
		}
	}
	_ = rows.Close()

	// Ordinary data survives: no row of an ordinary table was deleted.
	if n := rawCount(t, db, `SELECT COUNT(*) FROM actions WHERE agents_id = 'a1' AND id IN ('a1', 'a2')`); n != 2 {
		t.Errorf("actions rows left = %d, want 2", n)
	}
	if got := rawString(t, db, `SELECT data FROM actions WHERE id = 'a2'`); !strings.Contains(got, "ORDINARYACTION") {
		t.Errorf("ordinary actions field lost: %s", got)
	}
	if got := rawString(t, db, `SELECT data FROM resources WHERE resource_type = 'sites' AND id = 's1'`); !strings.Contains(got, "ORDINARYSITE") {
		t.Errorf("ordinary site field lost: %s", got)
	}
	if results, err := s.Search("ORDINARYSITE", 10); err != nil || len(results) == 0 {
		t.Errorf("the scrubbed site is no longer searchable: %v %v", results, err)
	}
	if n := rawCount(t, db, `SELECT COUNT(*) FROM resources_fts WHERE content LIKE '%ZEBRA%' OR content LIKE '%SITEREGTOKEN%'`); n != 0 {
		t.Errorf("the search index still holds %d entries with a credential", n)
	}
	if got := rawString(t, db, `SELECT data FROM uploads WHERE id = 'u1'`); !strings.Contains(got, "ORDINARYUPLOAD") {
		t.Errorf("ordinary upload field lost: %s", got)
	}
	if n := rawCount(t, db, `SELECT COUNT(*) FROM uploads WHERE id = 'u1' AND registration_token IS NULL AND api_key = ''`); n != 1 {
		t.Errorf("secret-named columns were not cleared (want NULL, and '' where NOT NULL)")
	}
	if got := rawString(t, db, `SELECT payload FROM learn_candidates WHERE derivation_signature = 'sig1'`); !strings.Contains(got, "ORDINARYLEARN") {
		t.Errorf("ordinary learn field lost: %s", got)
	}
	if got := rawString(t, db, `SELECT v FROM legacy_kv WHERE k = 'k1'`); !strings.Contains(got, "ORDINARYKV") {
		t.Errorf("ordinary WITHOUT ROWID field lost: %s", got)
	}

	// The never-stored partitions are gone, in any spelling; their typed
	// table stays (never dropped) but holds nothing.
	if n := rawCount(t, db, `SELECT COUNT(*) FROM resources WHERE lower(replace(resource_type, '_', '-')) = 'agents-passphrases'`); n != 0 {
		t.Errorf("agents-passphrases resources rows left: %d", n)
	}
	if n := rawCount(t, db, `SELECT COUNT(*) FROM sync_state WHERE resource_type = 'agents-passphrases'`); n != 0 {
		t.Errorf("agents-passphrases sync_state rows left: %d", n)
	}
	if n := rawCount(t, db, `SELECT COUNT(*) FROM agents_passphrases`); n != 0 {
		t.Errorf("never-stored typed table rows left: %d", n)
	}

	if _, err := db.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if leaks := secretStoreFileLeaks(t, dir, sentinels...); len(leaks) > 0 {
		t.Fatalf("credentials an older build stored survived the open:\n%s", strings.Join(leaks, "\n"))
	}
	for _, o := range []string{"ORDINARYACTION", "ORDINARYSITE", "ORDINARYUPLOAD", "ORDINARYLEARN", "ORDINARYKV"} {
		if len(secretStoreFileLeaks(t, dir, o)) == 0 {
			t.Errorf("ordinary value %q is no longer in the store", o)
		}
	}
}

// TestScrubRunsOncePerOldStore: the scrub is gated on the schema version. A
// store already at v14 is not rescanned (a row written straight into it with
// SQL keeps its secret-named field: the write paths drop those, so the gate
// costs nothing), and the same store stamped back to v13 is scrubbed again.
func TestScrubRunsOncePerOldStore(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "data.db")
	s, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	raw := openRawStore(t, dbPath)
	if v := rawUserVersion(t, raw); v != 14 {
		raw.Close()
		t.Fatalf("a fresh store is at v%d, want v14", v)
	}
	if _, err := raw.Exec(`INSERT INTO actions (id, agents_id, data) VALUES ('late', 'a1', '{"id":"late","passphrase":"AFTERUPGRADEPASS"}')`); err != nil {
		raw.Close()
		t.Fatal(err)
	}
	raw.Close()

	s, err = Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	got := rawString(t, s.DB(), `SELECT json_extract(data, '$.passphrase') FROM actions WHERE id = 'late'`)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if got != "AFTERUPGRADEPASS" {
		t.Fatalf("a v14 store was rescanned on open (passphrase = %q); the scrub must run once per old store", got)
	}

	raw = openRawStore(t, dbPath)
	if _, err := raw.Exec(`PRAGMA user_version = 13`); err != nil {
		raw.Close()
		t.Fatal(err)
	}
	raw.Close()
	s, err = Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	got = rawString(t, s.DB(), `SELECT json_extract(data, '$.passphrase') FROM actions WHERE id = 'late'`)
	v, _ := s.SchemaVersion()
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if got != "" || v != 14 {
		t.Fatalf("a v13 store was not scrubbed on open (passphrase = %q, version %d)", got, v)
	}
}

// TestScrubFailureLeavesStoreBelowV14: a scrub that fails part-way (an
// injected trigger aborts the rewrite of one table) fails the open and
// leaves the store exactly as it was, still below v14, so the next open
// scrubs it again (fail closed). Earlier steps of the same scrub roll back
// too: the scrub and the version stamp are one transaction.
func TestScrubFailureLeavesStoreBelowV14(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "data.db")
	writeOlderBuildStore(t, dbPath,
		olderBuildStmt{`INSERT INTO actions (id, agents_id, data) VALUES ('a1', 'a1', ?)`,
			[]any{`{"id":"a1","agents_id":"a1","passphrase":"` + codexRound2Passphrase + `"}`}},
		olderBuildStmt{`INSERT INTO resources (id, resource_type, data) VALUES ('11', 'agents-passphrases', '{"id":"11","passphrase":"PARTITIONPASS99"}')`, nil},
		olderBuildStmt{`CREATE TRIGGER scrub_fail BEFORE UPDATE ON actions BEGIN SELECT RAISE(ABORT, 'injected scrub failure'); END`, nil},
	)

	if s, err := Open(dbPath); err == nil {
		_ = s.Close()
		t.Fatal("the open succeeded although the scrub could not rewrite actions")
	} else if !strings.Contains(err.Error(), "injected scrub failure") {
		t.Fatalf("the open failed for another reason: %v", err)
	}

	raw := openRawStore(t, dbPath)
	if v := rawUserVersion(t, raw); v != 13 {
		raw.Close()
		t.Fatalf("a failed scrub left the store at v%d, want v13", v)
	}
	if got := rawString(t, raw, `SELECT json_extract(data, '$.passphrase') FROM actions WHERE id = 'a1'`); got != codexRound2Passphrase {
		raw.Close()
		t.Fatalf("a failed scrub partly committed: actions passphrase = %q", got)
	}
	if n := rawCount(t, raw, `SELECT COUNT(*) FROM resources WHERE resource_type = 'agents-passphrases'`); n != 1 {
		raw.Close()
		t.Fatalf("a failed scrub partly committed: %d partition rows left, want the 1 rolled back", n)
	}
	if _, err := raw.Exec(`DROP TRIGGER scrub_fail`); err != nil {
		raw.Close()
		t.Fatal(err)
	}
	raw.Close()

	s, err := Open(dbPath)
	if err != nil {
		t.Fatalf("the open after the failure was removed: %v", err)
	}
	defer s.Close()
	if v, _ := s.SchemaVersion(); v != 14 {
		t.Fatalf("version after the retried scrub = %d, want 14", v)
	}
	if got := rawString(t, s.DB(), `SELECT json_extract(data, '$.passphrase') FROM actions WHERE id = 'a1'`); got != "" {
		t.Fatalf("the retried scrub left the passphrase: %q", got)
	}
}
