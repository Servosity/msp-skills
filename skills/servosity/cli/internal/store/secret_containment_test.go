// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/servosity/handfixes.json under
// the "secret-family-never-persisted" entry. See docs/reprint-survival.md.

package store

import (
	"database/sql"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

const secretSentinel = "storesentinel7f3a"

// TestSecretFieldsAreNeverPersisted drives the batch and single-row write paths
// with the credential-bearing shapes this API returns and checks that no
// table, typed column or FTS index keeps the value, while the rest of each row
// is stored.
func TestSecretFieldsAreNeverPersisted(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer s.Close()

	batches := map[string][]json.RawMessage{
		"credentials": {json.RawMessage(`{"id":1,"name":"nas share","username":"svc","password":"` + secretSentinel + `-pw"}`)},
		"backups":     {json.RawMessage(`{"id":11,"name":"ACME-FS1","shadowprotect_keys":[{"product_key":"` + secretSentinel + `-pk","product_type":"Server"}]}`)},
		"resellers":   {json.RawMessage(`{"id":7,"name":"Partner","postmark_server_api_token":"` + secretSentinel + `-pm"}`)},
		"agent-login": {json.RawMessage(`{"id":3,"login":"agent","token":"` + secretSentinel + `-tok"}`)},
		"backup_sets": {json.RawMessage(`{"ID":"1600000000000","Name":"Nightly","LanPassword":"` + secretSentinel + `-lan","ApplicationSettings":{"EncryptionPassword":"` + secretSentinel + `-enc","Password":""}}`)},
	}
	for resource, items := range batches {
		if _, _, err := s.UpsertBatch(resource, items); err != nil {
			t.Fatalf("UpsertBatch(%s): %v", resource, err)
		}
	}
	if err := s.Upsert("companies", "4421", json.RawMessage(`{"id":4421,"name":"Acme Co","agent_provision_token_id":"`+secretSentinel+`-prov"}`)); err != nil {
		t.Fatalf("Upsert(companies): %v", err)
	}

	for _, refused := range []string{"backups-mfa-codes", "current-user-mfa-backup-codes", "current-user-start-mfa"} {
		_, _, err := s.UpsertBatch(refused, []json.RawMessage{json.RawMessage(`{"id":1,"codes":["` + secretSentinel + `"]}`)})
		if err == nil || !strings.Contains(err.Error(), "refusing to persist sensitive resource") {
			t.Fatalf("UpsertBatch(%s) error = %v, want a refusal", refused, err)
		}
	}

	assertNoSecretSentinel(t, s.DB())
	for _, keep := range []string{
		`SELECT COUNT(*) FROM resources WHERE resource_type = 'credentials' AND json_extract(data, '$.name') = 'nas share'`,
		`SELECT COUNT(*) FROM resources WHERE resource_type = 'backups' AND json_extract(data, '$.shadowprotect_keys[0].product_type') = 'Server'`,
		`SELECT COUNT(*) FROM resources WHERE resource_type = 'companies' AND json_extract(data, '$.name') = 'Acme Co'`,
	} {
		var n int
		if err := s.DB().QueryRow(keep).Scan(&n); err != nil || n != 1 {
			t.Fatalf("%s = %d (%v), want the row kept without its secret", keep, n, err)
		}
	}
}

// TestSchemaBelow13PurgesMirroredSecrets seeds a store the way 0.6.10 left it
// (schema 10, secrets mirrored in resources, typed columns and FTS) and checks
// the first read-write open removes them and keeps the rest.
func TestSchemaBelow13PurgesMirroredSecrets(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "data.db")
	seed, err := Open(dbPath)
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	if err := seed.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}
	raw, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open raw store: %v", err)
	}
	stmts := []string{
		`INSERT INTO resources (id, resource_type, data) VALUES ('1', 'credentials', '{"id":1,"name":"nas share","password":"` + secretSentinel + `-pw"}')`,
		`INSERT INTO resources (id, resource_type, data) VALUES ('1', 'backups-mfa-codes', '{"id":1,"mfa_codes":[{"prefix":"ab","code":"` + secretSentinel + `"}]}')`,
		`INSERT INTO resources (id, resource_type, data) VALUES ('me', 'current-user-start-mfa', '{"secret":"` + secretSentinel + `-totp"}')`,
		`INSERT INTO resources (id, resource_type, data) VALUES ('11', 'backups', '{"id":11,"name":"ACME-FS1","shadowprotect_keys":[{"product_key":"` + secretSentinel + `-pk"}]}')`,
		`INSERT INTO resources (id, resource_type, data) VALUES ('safe', 'issues', '{"id":"safe","name":"preserve me"}')`,
		`INSERT INTO resources (id, resource_type, data) VALUES ('1600000000000', 'backup_sets', '{"ID":"1600000000000","LanPassword":"` + secretSentinel + `-lan","ApplicationSettings":{"EncryptionPassword":"` + secretSentinel + `-enc"}}')`,
		`INSERT INTO credentials (id, data, name, password) VALUES ('1', '{"id":1,"name":"nas share","password":"` + secretSentinel + `-pw"}', 'nas share', '` + secretSentinel + `-pw')`,
		`INSERT INTO resellers (id, data, postmark_server_api_token) VALUES ('7', '{"id":7,"postmark_server_api_token":"` + secretSentinel + `-pm"}', '` + secretSentinel + `-pm')`,
		`INSERT INTO resources_fts (rowid, id, resource_type, content) VALUES (424242, '11', 'backups', '` + secretSentinel + ` pk')`,
		`PRAGMA user_version = 10`,
	}
	for _, stmt := range stmts {
		if _, err := raw.Exec(stmt); err != nil {
			raw.Close()
			t.Fatalf("seed v10 store (%s): %v", stmt, err)
		}
	}
	if err := raw.Close(); err != nil {
		t.Fatalf("close raw store: %v", err)
	}

	for pass := 1; pass <= 2; pass++ {
		s, err := Open(dbPath)
		if err != nil {
			t.Fatalf("open pass %d: %v", pass, err)
		}
		assertNoSecretSentinel(t, s.DB())
		for _, keep := range []string{
			`SELECT COUNT(*) FROM resources WHERE id = 'safe' AND resource_type = 'issues'`,
			`SELECT COUNT(*) FROM resources WHERE id = '11' AND resource_type = 'backups' AND json_extract(data, '$.name') = 'ACME-FS1'`,
			`SELECT COUNT(*) FROM credentials WHERE id = '1' AND name = 'nas share'`,
		} {
			var n int
			if err := s.DB().QueryRow(keep).Scan(&n); err != nil || n != 1 {
				s.Close()
				t.Fatalf("pass %d: %s = %d (%v), want 1", pass, keep, n, err)
			}
		}
		for _, gone := range []string{"backups-mfa-codes", "current-user-start-mfa"} {
			var n int
			if err := s.DB().QueryRow(`SELECT COUNT(*) FROM resources WHERE resource_type = ?`, gone).Scan(&n); err != nil || n != 0 {
				s.Close()
				t.Fatalf("pass %d: %d %s rows left (%v)", pass, n, gone, err)
			}
		}
		if err := s.Close(); err != nil {
			t.Fatalf("close pass %d: %v", pass, err)
		}
	}
}

// assertNoSecretSentinel scans every ordinary table's text and the resources
// FTS index for the sentinel.
func assertNoSecretSentinel(t *testing.T, db *sql.DB) {
	t.Helper()
	rows, err := db.Query(`SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%' AND sql NOT LIKE 'CREATE VIRTUAL TABLE%'`)
	if err != nil {
		t.Fatalf("list tables: %v", err)
	}
	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			t.Fatalf("scan table: %v", err)
		}
		tables = append(tables, name)
	}
	rows.Close()
	for _, table := range tables {
		cols, err := db.Query(`SELECT name FROM pragma_table_info(?)`, table)
		if err != nil {
			t.Fatalf("table_info %s: %v", table, err)
		}
		var names []string
		for cols.Next() {
			var c string
			if err := cols.Scan(&c); err != nil {
				cols.Close()
				t.Fatalf("scan column: %v", err)
			}
			names = append(names, c)
		}
		cols.Close()
		for _, c := range names {
			var n int
			q := `SELECT COUNT(*) FROM "` + table + `" WHERE CAST("` + c + `" AS TEXT) LIKE '%` + secretSentinel + `%'`
			if err := db.QueryRow(q).Scan(&n); err != nil {
				t.Fatalf("%s: %v", q, err)
			}
			if n != 0 {
				t.Fatalf("%s.%s still holds the secret sentinel (%d rows)", table, c, n)
			}
		}
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM resources_fts WHERE resources_fts MATCH ?`, `"`+secretSentinel+`"`).Scan(&n); err != nil {
		t.Fatalf("FTS query: %v", err)
	}
	if n != 0 {
		t.Fatalf("resources_fts still indexes the secret sentinel (%d rows)", n)
	}
}
