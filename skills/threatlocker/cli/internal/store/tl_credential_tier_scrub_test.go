// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored: a store written by an earlier binary already holds
// credential-tier values. `organizations auth-key` wrote its response through
// to the store, so the organizations row carries authKey in organizations.data,
// in organizations.auth_key, in resources.data and in the resources_fts leaf
// index, where the MCP `sql` and `search` tools read it. The next writable open
// must remove every copy while keeping the organization row itself.

package store

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

func TestLegacyCredentialTierValuesAreScrubbedOnOpen(t *testing.T) {
	const authKey = "SENTINEL-legacy-authkey-feedface"
	const installKey = "SENTINEL-legacy-installkey-deadbeef"
	dbPath := filepath.Join(t.TempDir(), "legacy.db")

	db, err := OpenWithContext(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	// What the pre-fix write-through stored: a synced organization row, then
	// the auth-key response merged over it, and a computer row carrying the
	// install-info fields.
	if _, _, err := db.UpsertBatch("organizations", []json.RawMessage{
		json.RawMessage(`{"organizationId":"org-legacy","name":"Legacy Tenant"}`),
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := db.UpsertBatch("organizations", []json.RawMessage{
		json.RawMessage(`{"organizationId":"org-legacy","authKey":"` + authKey + `"}`),
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := db.UpsertBatch("computers", []json.RawMessage{
		json.RawMessage(`{"computerId":"pc-legacy","computerName":"Legacy PC","installKey":"` + installKey + `","downloadUrl":"https://download.example.test/agent?key=` + installKey + `"}`),
	}); err != nil {
		t.Fatal(err)
	}
	if n := countSecretRows(t, db, authKey) + countSecretRows(t, db, installKey); n == 0 {
		t.Fatal("seeding did not reach the store, so the scrub below proves nothing")
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	db, err = OpenWithContext(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, secret := range []string{authKey, installKey} {
		if n := countSecretRows(t, db, secret); n != 0 {
			t.Errorf("%d SQL-visible cop(ies) of %s survived reopening the store", n, secret)
		}
		results, err := db.Search(`"`+strings.TrimPrefix(secret, "SENTINEL-")+`"`, 50)
		if err != nil {
			t.Fatalf("search: %v", err)
		}
		for _, r := range results {
			if strings.Contains(string(r), secret) {
				t.Errorf("full-text search returns the credential: %s", r)
			}
		}
	}
	var name string
	if err := db.DB().QueryRow(`SELECT json_extract(data, '$.name') FROM organizations WHERE organization_id = 'org-legacy'`).Scan(&name); err != nil || name != "Legacy Tenant" {
		t.Errorf("the scrub destroyed the organization row itself: name=%q err=%v", name, err)
	}
}

// The raw response bytes an earlier binary stored need not spell the key
// literally: JSON lets "authKey" arrive as "authKey" (or in another
// case), and SQLite decodes it, so json_extract(data, '$.authKey') still
// reads the value. The scrub must find it by what SQLite reads, not by the
// raw spelling.
func TestLegacyCredentialTierValuesWithEscapedKeysAreScrubbed(t *testing.T) {
	const secret = "test-secret-escaped-key"
	dbPath := filepath.Join(t.TempDir(), "escaped.db")
	db, err := OpenWithContext(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	bs := string(rune(92)) // a JSON backslash, built at runtime so no tool decodes it early
	rows := []struct{ resourceType, id, data string }{
		{"organizations", "org-esc", `{"organizationId":"org-esc","name":"Escaped Tenant","` + bs + `u0061uthKey":"` + secret + `"}`},
		{"organizations", "org-case", `{"organizationId":"org-case","AUTHKEY":"` + secret + `"}`},
		{"organizations", "org-nested", `{"organizationId":"org-nested","data":{"` + bs + `u0061uth` + bs + `u004Bey":"` + secret + `"}}`},
		{"computers", "pc-esc", `{"computerId":"pc-esc","` + bs + `u0069nstallKey":"` + secret + `","download` + bs + `u0055rl":"https://dl.example.test/?k=` + secret + `"}`},
	}
	for _, r := range rows {
		if _, err := db.DB().Exec(`INSERT INTO resources (id, resource_type, data) VALUES (?, ?, ?)`, r.id, r.resourceType, r.data); err != nil {
			t.Fatal(err)
		}
		if _, err := db.DB().Exec(`INSERT INTO resources_fts (rowid, id, resource_type, content) VALUES (?, ?, ?, ?)`, ftsRowID(r.resourceType, r.id), r.id, r.resourceType, secret); err != nil {
			t.Fatal(err)
		}
		table := map[string]string{"organizations": "organizations", "computers": "computers"}[r.resourceType]
		if _, err := db.DB().Exec(`INSERT INTO "`+table+`" (id, data) VALUES (?, ?)`, r.id, r.data); err != nil {
			t.Fatal(err)
		}
	}
	var readable int
	if err := db.DB().QueryRow(`SELECT count(*) FROM resources WHERE json_extract(data, '$.authKey') = ? OR json_extract(data, '$.installKey') = ?`, secret, secret).Scan(&readable); err != nil || readable == 0 {
		t.Fatalf("seeded escaped keys are not SQL-readable (n=%d, err=%v), so the check below proves nothing", readable, err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	db, err = OpenWithContext(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if n := countSecretRows(t, db, secret); n != 0 {
		t.Errorf("%d SQL-visible cop(ies) of an escaped-key credential survived reopening the store", n)
	}
	var name string
	if err := db.DB().QueryRow(`SELECT json_extract(data, '$.name') FROM resources WHERE id = 'org-esc'`).Scan(&name); err != nil || name != "Escaped Tenant" {
		t.Errorf("the scrub destroyed the row itself: name=%q err=%v", name, err)
	}
}

// countSecretRows counts every place the MCP sql/search tools could read the
// value from: every column of the typed tables (whatever columns the schema
// has, so a dropped column cannot hide a copy and an added one cannot escape
// the count), the generic resources table, and the resources_fts content.
func countSecretRows(t *testing.T, db *Store, secret string) int {
	t.Helper()
	total := 0
	for _, table := range []string{"organizations", "computers", "resources"} {
		rows, err := db.DB().Query(`SELECT * FROM "` + table + `"`)
		if err != nil {
			t.Fatalf("reading %s: %v", table, err)
		}
		cols, _ := rows.Columns()
		for rows.Next() {
			vals := make([]any, len(cols))
			ptrs := make([]any, len(cols))
			for i := range vals {
				ptrs[i] = &vals[i]
			}
			if err := rows.Scan(ptrs...); err != nil {
				t.Fatal(err)
			}
			for _, v := range vals {
				if strings.Contains(fmt.Sprint(v), secret) || strings.Contains(fmt.Sprintf("%s", v), secret) {
					total++
					break
				}
			}
		}
		_ = rows.Close()
	}
	var n int
	if err := db.DB().QueryRow(`SELECT count(*) FROM resources_fts WHERE content LIKE ?`, "%"+secret+"%").Scan(&n); err != nil {
		t.Fatalf("resources_fts: %v", err)
	}
	return total + n
}
