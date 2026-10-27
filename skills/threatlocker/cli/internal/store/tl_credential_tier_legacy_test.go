// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package store

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

const tlLegacySentinel = "test-secret-legacy-install-key"

// seedV0312CredentialStore writes a store shaped like one the released v0.3.12
// binary (schema version 9) leaves behind after `organizations auth-key` and
// `computers install-info`: the typed auth_key / install_key / download_url
// columns hold the keys, the typed rows' JSON and the generic resources rows
// carry them, and the resources_fts index holds them as searchable text.
func seedV0312CredentialStore(t *testing.T) string {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "data.db")
	s, err := OpenWithContext(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	db := s.DB()
	for _, c := range []struct{ table, column string }{
		{"organizations", "auth_key"}, {"computers", "install_key"}, {"computers", "download_url"},
	} {
		var n int
		if err := db.QueryRow(`SELECT count(*) FROM pragma_table_info(?) WHERE name = ?`, c.table, c.column).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n == 0 {
			if _, err := db.Exec(fmt.Sprintf(`ALTER TABLE %q ADD COLUMN %q TEXT`, c.table, c.column)); err != nil {
				t.Fatalf("adding legacy column %s.%s: %v", c.table, c.column, err)
			}
		}
	}
	orgJSON := `{"organizationId":"org-1","name":"Tenant 1","authKey":"` + tlLegacySentinel + `"}`
	compJSON := `{"computerId":"c1","computerName":"PC-1","installKey":"` + tlLegacySentinel + `","downloadUrl":"https://download.example.test/a?k=` + tlLegacySentinel + `"}`
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO organizations (id, data, organization_id, name, auth_key) VALUES ('org-1', ?, 'org-1', 'Tenant 1', ?)`, []any{orgJSON, tlLegacySentinel}},
		{`INSERT INTO computers (id, data, computer_id, computer_name, organization_id, install_key, download_url) VALUES ('c1', ?, 'c1', 'PC-1', 'org-1', ?, ?)`, []any{compJSON, tlLegacySentinel, "https://download.example.test/a?k=" + tlLegacySentinel}},
		{`INSERT INTO resources (id, resource_type, data) VALUES ('org-1', 'organizations', ?)`, []any{orgJSON}},
		{`INSERT INTO resources (id, resource_type, data) VALUES ('c1', 'computers', ?)`, []any{compJSON}},
		{`DELETE FROM resources_fts WHERE rowid IN (?, ?)`, []any{ftsRowID("organizations", "org-1"), ftsRowID("computers", "c1")}},
		{`INSERT INTO resources_fts (rowid, id, resource_type, content) VALUES (?, 'org-1', 'organizations', ?)`, []any{ftsRowID("organizations", "org-1"), "Tenant 1 " + tlLegacySentinel}},
		{`INSERT INTO resources_fts (rowid, id, resource_type, content) VALUES (?, 'c1', 'computers', ?)`, []any{ftsRowID("computers", "c1"), "PC-1 " + tlLegacySentinel}},
		{`PRAGMA user_version = 9`, nil},
	} {
		if _, err := db.Exec(q.sql, q.args...); err != nil {
			t.Fatalf("seed %q: %v", q.sql, err)
		}
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	return dbPath
}

// dumpStoreText reads everything a read-only SQL or search caller can read
// from the tables the credential tier touches, as one string.
func dumpStoreText(t *testing.T, db *sql.DB) string {
	t.Helper()
	var b strings.Builder
	for _, table := range []string{"organizations", "computers", "resources"} {
		rows, err := db.Query(`SELECT * FROM "` + table + `"`)
		if err != nil {
			t.Fatalf("reading %s: %v", table, err)
		}
		cols, _ := rows.Columns()
		b.WriteString(table + ": " + strings.Join(cols, ",") + "\n")
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
				fmt.Fprintf(&b, "%s|", v)
			}
			b.WriteString("\n")
		}
		_ = rows.Close()
	}
	rows, err := db.Query(`SELECT content FROM resources_fts WHERE resources_fts MATCH ?`, `"`+tlLegacySentinel+`"`)
	if err != nil {
		t.Fatalf("searching resources_fts: %v", err)
	}
	for rows.Next() {
		var c string
		_ = rows.Scan(&c)
		b.WriteString("fts: " + c + "\n")
	}
	_ = rows.Close()
	return b.String()
}

// A store the released v0.3.12 binary wrote holds the installation keys. A
// READ-ONLY open (what the MCP sql, search and export tools use) must already
// see a store with the keys gone and the key columns dropped: the read-only
// open upgrades a store this binary has not migrated before answering.
func TestReadOnlyOpenOfAV0312StoreNeverExposesInstallKeys(t *testing.T) {
	dbPath := seedV0312CredentialStore(t)

	ro, err := OpenReadOnly(dbPath)
	if err != nil {
		t.Fatalf("read-only open of a v0.3.12 store: %v", err)
	}
	defer ro.Close()
	text := dumpStoreText(t, ro.DB())
	if strings.Contains(text, tlLegacySentinel) {
		t.Fatalf("a read-only open of a v0.3.12 store exposes the installation key:\n%s", text)
	}
	for _, c := range []struct{ table, column string }{
		{"organizations", "auth_key"}, {"computers", "install_key"}, {"computers", "download_url"},
	} {
		var n int
		if err := ro.DB().QueryRow(`SELECT count(*) FROM pragma_table_info(?) WHERE name = ?`, c.table, c.column).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Fatalf("%s.%s is still in the schema after the upgrade", c.table, c.column)
		}
	}
	v, err := ro.SchemaVersion()
	if err != nil {
		t.Fatal(err)
	}
	if v != StoreSchemaVersion {
		t.Fatalf("user_version %d after the upgrade, want %d", v, StoreSchemaVersion)
	}
	// The rows themselves survive: only the keys go.
	if !strings.Contains(text, "Tenant 1") || !strings.Contains(text, "PC-1") {
		t.Fatalf("the upgrade dropped the rows, not only the keys:\n%s", text)
	}
}

// The upgrade must fail closed: when a store this binary has not migrated
// cannot be opened for writing, the read-only open refuses rather than
// answering from a store that may still hold a key.
func TestReadOnlyOpenRefusesALegacyStoreItCannotUpgrade(t *testing.T) {
	dbPath := seedV0312CredentialStore(t)
	restore := legacyUpgradeOpenHook
	legacyUpgradeOpenHook = func(context.Context, string) (*Store, error) {
		return nil, fmt.Errorf("simulated: store is on read-only media")
	}
	t.Cleanup(func() { legacyUpgradeOpenHook = restore })

	ro, err := OpenReadOnly(dbPath)
	if err == nil {
		text := dumpStoreText(t, ro.DB())
		_ = ro.Close()
		t.Fatalf("read-only open answered from a store it could not upgrade:\n%s", text)
	}
	if !strings.Contains(err.Error(), "installation key") {
		t.Fatalf("refusal does not say why: %v", err)
	}
}

// A store this binary already migrated opens read-only without a writable
// open (the read-only path stays read-only once the upgrade has happened).
func TestReadOnlyOpenOfACurrentStoreDoesNotReopenForWriting(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "data.db")
	s, err := OpenWithContext(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
	restore := legacyUpgradeOpenHook
	legacyUpgradeOpenHook = func(context.Context, string) (*Store, error) {
		t.Fatal("a current store was reopened for writing")
		return nil, nil
	}
	t.Cleanup(func() { legacyUpgradeOpenHook = restore })
	ro, err := OpenReadOnly(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	_ = ro.Close()
}

// An unstamped database (user_version 0) that still carries a key is upgraded
// too; one with no trace of a key is left alone (the generated
// TestOpenReadOnly_DeleteModeDBDoesNotWrite pins that half).
func TestReadOnlyOpenOfAnUnstampedStoreWithAKeyIsUpgraded(t *testing.T) {
	dbPath := seedV0312CredentialStore(t)
	raw, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`PRAGMA user_version = 0`); err != nil {
		t.Fatal(err)
	}
	_ = raw.Close()

	ro, err := OpenReadOnly(dbPath)
	if err != nil {
		t.Fatalf("read-only open of an unstamped store holding a key: %v", err)
	}
	defer ro.Close()
	if text := dumpStoreText(t, ro.DB()); strings.Contains(text, tlLegacySentinel) {
		t.Fatalf("a read-only open of an unstamped store exposes the installation key:\n%s", text)
	}
}
