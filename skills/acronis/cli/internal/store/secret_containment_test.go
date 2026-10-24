// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/acronis/handfixes.json under
// the "credential-fields-never-persisted" entry. See docs/reprint-survival.md.

package store

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// credentialSentinels are the credential values the rows below carry. None may
// reach the SQLite file, its WAL, or any column.
var credentialSentinels = []string{
	"test-secret-aaaaaaaaaaaa",    // clients create answer (batch path)
	"SingleSecretSentinel0002",    // clients row on the single-row path
	"IdpAccessSentinel0003",       // idp request-token answer
	"IdpRefreshSentinel0004",      // idp request-token answer
	"IdpIdTokenSentinel0005",      // idp request-token answer
	"NestedPasswordSentinel0006",  // generic row, nested object
	"NestedApiTokenSentinel0007",  // generic row, object inside an array
	"NestedClientSecretSent0008",  // generic row, camelCase, two levels down
	"GenericAccessSentinel0009",   // untyped resource via Upsert
	"KeyAsIdentitySentinel0010",   // a row whose only identity is "key"
	"MergedSecretSentinel0011",    // a second write merged onto a stored row
	"FtsOnlySecretSentinel0012",   // a token in a row that is searched
	"WriteThroughSecretSent0013",  // an envelope-unwrapped detail answer
	"PascalCasePasswordSent0014",  // PascalCase spelling
	"UpperSnakeTokenSentinel0015", // UPPER_SNAKE spelling
}

// TestCredentialFieldsAreNeverPersisted drives every store write path with the
// credential-bearing shapes Acronis returns (clients create, idp
// request-token, nested secrets in an ordinary row) and checks that no table,
// typed column, FTS index, database file or WAL keeps a credential, while the
// ordinary fields of each row are stored.
func TestCredentialFieldsAreNeverPersisted(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "data.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer s.Close()

	const tenant = "00000000-0000-0000-0000-000000000001"
	mustBatch := func(resource string, items ...string) {
		t.Helper()
		raw := make([]json.RawMessage, len(items))
		for i, item := range items {
			raw[i] = json.RawMessage(item)
		}
		if _, _, err := s.UpsertBatch(resource, raw); err != nil {
			t.Fatalf("UpsertBatch(%s): %v", resource, err)
		}
	}

	// clients create answer, the documented 201 body.
	mustBatch("clients", `{"id":"c-1","client_id":"new-client","client_secret":"test-secret-aaaaaaaaaaaa","tenant_id":"`+tenant+`","type":"api_client","status":"enabled"}`)
	// The single-row path (sync of a non-array answer).
	if err := s.UpsertClients(json.RawMessage(`{"client_id":"single-client","clientSecret":"SingleSecretSentinel0002","tenant_id":"` + tenant + `","status":"enabled"}`)); err != nil {
		t.Fatalf("UpsertClients: %v", err)
	}
	// A second write to the same row merges onto what is stored.
	mustBatch("clients", `{"client_id":"new-client","client_secret":"MergedSecretSentinel0011","status":"disabled"}`)

	// idp request-token answers are refused whole, with or without an id.
	idp := `{"id":"tok-1","access_token":"IdpAccessSentinel0003","refresh_token":"IdpRefreshSentinel0004","id_token":"IdpIdTokenSentinel0005","token_type":"bearer","expires_in":3600}`
	if _, _, err := s.UpsertBatch("idp", []json.RawMessage{json.RawMessage(idp)}); err == nil {
		t.Error("UpsertBatch(idp) stored a token answer")
	}
	if err := s.UpsertIdp(json.RawMessage(`{"access_token":"IdpAccessSentinel0003","token_type":"bearer"}`)); err == nil {
		t.Error("UpsertIdp stored a token answer")
	}

	// Ordinary rows with credentials nested at every depth and spelling.
	mustBatch("tenants", `{"id":"t-1","name":"Northwind IT","kind":"customer","contact":{"login":"admin","password":"NestedPasswordSentinel0006"},"integrations":[{"name":"psa","api_token":"NestedApiTokenSentinel0007","settings":{"label":"psa-link","clientSecret":"NestedClientSecretSent0008"}}],"Branding":{"SmtpPassword":"PascalCasePasswordSent0014","SMTP_TOKEN":"UpperSnakeTokenSentinel0015"}}`)
	mustBatch("task-manager", `{"id":"a-1","state":"completed","result":{"code":"ok"},"tenant":{"uuid":"`+tenant+`"},"policy":{"id":"p-1"},"session_token":"FtsOnlySecretSentinel0012"}`)
	if err := s.Upsert("reports", "r-1", json.RawMessage(`{"id":"r-1","name":"Monthly usage","delivery":{"access_token":"GenericAccessSentinel0009"}}`)); err != nil {
		t.Fatalf("Upsert(reports): %v", err)
	}
	// A row whose only identity-shaped field is a credential is not stored
	// under it.
	if _, _, err := s.UpsertBatch("widgets", []json.RawMessage{json.RawMessage(`{"key":"KeyAsIdentitySentinel0010"}`)}); err != nil {
		t.Fatalf("UpsertBatch(widgets): %v", err)
	}
	// An envelope-wrapped detail answer is unwrapped before the write.
	mustBatch("users", `{"user":{"id":"u-1","login":"jdoe","tenants_id":"`+tenant+`","auth":{"password":"WriteThroughSecretSent0013"}}}`)

	assertNoCredentialAnywhere(t, s.DB(), dir)

	// Ordinary fields stay, in the generic row and the typed columns alike.
	for _, keep := range []struct{ query, want string }{
		{`SELECT client_id FROM clients WHERE client_id = 'new-client'`, "new-client"},
		{`SELECT tenant_id FROM clients WHERE client_id = 'new-client'`, tenant},
		{`SELECT type FROM clients WHERE client_id = 'new-client'`, "api_client"},
		{`SELECT status FROM clients WHERE client_id = 'new-client'`, "disabled"},
		{`SELECT json_extract(data, '$.id') FROM resources WHERE resource_type = 'clients' AND id = 'new-client'`, "c-1"},
		{`SELECT client_id FROM clients WHERE client_id = 'single-client'`, "single-client"},
		{`SELECT json_extract(data, '$.name') FROM resources WHERE resource_type = 'tenants' AND id = 't-1'`, "Northwind IT"},
		{`SELECT json_extract(data, '$.contact.login') FROM resources WHERE resource_type = 'tenants' AND id = 't-1'`, "admin"},
		{`SELECT json_extract(data, '$.integrations[0].settings.label') FROM resources WHERE resource_type = 'tenants' AND id = 't-1'`, "psa-link"},
		{`SELECT name FROM tenants WHERE id = 't-1'`, "Northwind IT"},
		{`SELECT result_code FROM task_manager WHERE id = 'a-1'`, "ok"},
		{`SELECT tenant_id FROM task_manager WHERE id = 'a-1'`, tenant},
		{`SELECT json_extract(data, '$.name') FROM resources WHERE resource_type = 'reports' AND id = 'r-1'`, "Monthly usage"},
		{`SELECT json_extract(data, '$.login') FROM resources WHERE resource_type = 'users'`, "jdoe"},
	} {
		var got sql.NullString
		if err := s.DB().QueryRow(keep.query).Scan(&got); err != nil || got.String != keep.want {
			t.Errorf("%s = %q (%v), want %q", keep.query, got.String, err, keep.want)
		}
	}
	var secretCol sql.NullString
	if err := s.DB().QueryRow(`SELECT client_secret FROM clients WHERE client_id = 'new-client'`).Scan(&secretCol); err != nil || secretCol.Valid {
		t.Errorf("clients.client_secret = %q (valid=%v, %v), want NULL", secretCol.String, secretCol.Valid, err)
	}
	if hits, err := s.Search("Northwind", 10); err != nil || len(hits) != 1 {
		t.Errorf("search for an ordinary value found %d rows (%v), want 1", len(hits), err)
	}
	for _, sentinel := range credentialSentinels {
		if hits, err := s.Search(sentinel, 10); err != nil || len(hits) != 0 {
			t.Errorf("search for %s found %d rows (%v)", sentinel, len(hits), err)
		}
	}
	var widgets int
	if err := s.DB().QueryRow(`SELECT COUNT(*) FROM resources WHERE resource_type = 'widgets'`).Scan(&widgets); err != nil || widgets != 0 {
		t.Errorf("a row keyed only by a credential was stored: %d (%v)", widgets, err)
	}

	// After close, the checkpointed file must still hold none of them.
	if err := s.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	assertNoCredentialInFiles(t, dir)
}

// TestNeverPersistedResourceMatchesIdpOnly pins which resource types are
// refused whole.
func TestNeverPersistedResourceMatchesIdpOnly(t *testing.T) {
	for _, rt := range []string{"idp", "IDP", "idp-token", "idp_tokens", "idp/token"} {
		if !neverPersistedResource(rt) {
			t.Errorf("%q must be refused", rt)
		}
	}
	for _, rt := range []string{"clients", "tenants", "id-policies", "users", "task-manager", ""} {
		if neverPersistedResource(rt) {
			t.Errorf("%q must not be refused", rt)
		}
	}
}

// assertNoCredentialAnywhere checks every column of every table, then the
// database file and its WAL.
func assertNoCredentialAnywhere(t *testing.T, db *sql.DB, dir string) {
	t.Helper()
	rows, err := db.Query(`SELECT name FROM sqlite_master WHERE type = 'table'`)
	if err != nil {
		t.Fatalf("list tables: %v", err)
	}
	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan table: %v", err)
		}
		tables = append(tables, name)
	}
	_ = rows.Close()
	if len(tables) == 0 {
		t.Fatal("no tables listed; the column check proves nothing")
	}
	for _, table := range tables {
		r, err := db.Query(fmt.Sprintf(`SELECT * FROM "%s"`, table))
		if err != nil {
			continue // a virtual table without a usable full scan
		}
		cols, _ := r.Columns()
		for r.Next() {
			cells := make([]any, len(cols))
			ptrs := make([]any, len(cols))
			for i := range cells {
				ptrs[i] = &cells[i]
			}
			if err := r.Scan(ptrs...); err != nil {
				t.Fatalf("scan %s: %v", table, err)
			}
			for i, cell := range cells {
				var text string
				switch v := cell.(type) {
				case []byte:
					text = string(v)
				case string:
					text = v
				default:
					continue
				}
				for _, sentinel := range credentialSentinels {
					if strings.Contains(text, sentinel) {
						t.Errorf("%s.%s holds %s", table, cols[i], sentinel)
					}
				}
			}
		}
		_ = r.Close()
	}
	assertNoCredentialInFiles(t, dir)
}

// assertNoCredentialInFiles reads the database file, WAL and any other file
// the store wrote beside it. The ordinary value new-client must be in those
// same bytes, or the check proves nothing.
func assertNoCredentialInFiles(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	ordinary := false
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		body, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		for _, sentinel := range credentialSentinels {
			if bytes.Contains(body, []byte(sentinel)) {
				t.Errorf("%s holds %s", e.Name(), sentinel)
			}
		}
		if bytes.Contains(body, []byte("new-client")) {
			ordinary = true
		}
	}
	if !ordinary {
		t.Error("no store file holds the ordinary value new-client; the byte check proves nothing")
	}
}
