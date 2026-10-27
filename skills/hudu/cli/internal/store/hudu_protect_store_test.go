// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-fix (handfixes.json vault-secrets-withheld): proves the store never
// keeps a Hudu tenant secret. A row written by an older binary (0.1.16 and
// before) still holds the vault entry's password and TOTP seed, or a
// password-type asset field value; the write funnel must not carry it forward
// through the keep-richer merge, and opening an upgraded store must scrub it.
// These tests use only store APIs that predate the fix, so they also run (and
// fail) against the unfixed store.

package store

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	protectOldPW  = "SENTINEL-OLD-PW"
	protectOldOTP = "SENTINEL-OLD-OTP"
	protectOldAF  = "SENTINEL-OLD-AF"
)

var protectSentinels = []string{protectOldPW, protectOldOTP, protectOldAF}

// seedLegacyRow writes a row the way an older binary left it: the generic
// resources row, the typed table's data column, and the full-text entry, all
// holding the plaintext.
func seedLegacyRow(t *testing.T, db *sql.DB, resourceType, typedTable, id, data string) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO resources (id, resource_type, data) VALUES (?, ?, ?)`, id, resourceType, data); err != nil {
		t.Fatalf("seed resources %s/%s: %v", resourceType, id, err)
	}
	if typedTable != "" {
		if _, err := db.Exec(`INSERT INTO "`+typedTable+`" (id, data) VALUES (?, ?)`, id, data); err != nil {
			t.Fatalf("seed %s/%s: %v", typedTable, id, err)
		}
	}
	if _, err := db.Exec(`INSERT INTO resources_fts (rowid, id, resource_type, content) VALUES (?, ?, ?, ?)`, ftsRowID(resourceType, id), id, resourceType, data); err != nil {
		t.Fatalf("seed resources_fts %s/%s: %v", resourceType, id, err)
	}
}

// assertNoProtectedSentinel reads every place MCP `sql` and `search` can see.
func assertNoProtectedSentinel(t *testing.T, db *sql.DB) {
	t.Helper()
	for _, q := range []string{
		`SELECT data FROM resources`,
		`SELECT data FROM asset_passwords`,
		`SELECT data FROM assets`,
		`SELECT content FROM resources_fts`,
	} {
		rows, err := db.Query(q)
		if err != nil {
			t.Fatalf("%s: %v", q, err)
		}
		for rows.Next() {
			var v string
			if err := rows.Scan(&v); err != nil {
				t.Fatalf("%s scan: %v", q, err)
			}
			for _, s := range protectSentinels {
				if strings.Contains(v, s) {
					t.Errorf("%s returned a stored secret %s: %s", q, s, v)
				}
			}
		}
		if err := rows.Err(); err != nil {
			t.Fatalf("%s rows: %v", q, err)
		}
		rows.Close()
	}
}

func legacyVaultRow(id string) string {
	return `{"id":` + id + `,"name":"vpn","password":"` + protectOldPW + `","otp_secret":"` + protectOldOTP + `"}`
}

// Codex r2 P1 #1 (funnel): the next sync's filtered row merges with the old
// one; before the fix the merge kept the password and MCP sql
// `SELECT data FROM asset_passwords WHERE id='7'` returned it.
func TestProtectFunnelScrubsStoredVaultSecretOnMerge(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()
	db := s.DB()
	seedLegacyRow(t, db, "asset-passwords", "asset_passwords", "7", legacyVaultRow("7"))
	seedLegacyRow(t, db, "asset-passwords", "asset_passwords", "8", legacyVaultRow("8"))

	if err := s.UpsertAssetPasswords(json.RawMessage(`{"id":7,"name":"vpn","username":"u"}`)); err != nil {
		t.Fatalf("UpsertAssetPasswords: %v", err)
	}
	if _, _, err := s.UpsertBatch("asset-passwords", []json.RawMessage{json.RawMessage(`{"id":8,"name":"vpn","username":"u"}`)}); err != nil {
		t.Fatalf("UpsertBatch: %v", err)
	}

	var data string
	if err := db.QueryRow(`SELECT data FROM asset_passwords WHERE id='7'`).Scan(&data); err != nil {
		t.Fatalf("select asset_passwords 7: %v", err)
	}
	if strings.Contains(data, protectOldPW) || strings.Contains(data, protectOldOTP) {
		t.Errorf("SELECT data FROM asset_passwords WHERE id='7' returned the stored secret: %s", data)
	}
	if !strings.Contains(data, `"username":"u"`) {
		t.Fatalf("the merged row lost its metadata: %s", data)
	}
	assertNoProtectedSentinel(t, db)
}

// Codex r2 P1 #1 (migration): an install upgraded from 0.1.16 opens a store
// whose rows still hold the plaintext. Opening it must scrub every vault row
// and every password-type asset field value, in the generic table, the typed
// tables and the full-text index, including a parent-sharded write-through
// partition, and stamp the new schema version.
func TestProtectMigrationScrubsUpgradedStore(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "data.db")
	s, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	db := s.DB()
	if err := s.UpsertAssetLayouts(json.RawMessage(`{"id":3,"name":"Server","fields":[{"label":"Admin Password","field_type":"Password"},{"label":"Hostname","field_type":"Text"}]}`)); err != nil {
		t.Fatalf("UpsertAssetLayouts: %v", err)
	}
	asset := func(id string) string {
		return `{"id":` + id + `,"asset_layout_id":3,"name":"dc","fields":[{"id":10,"label":"Admin Password","value":"` + protectOldAF + `","position":1},{"id":11,"label":"Hostname","value":"dc01","position":2}]}`
	}
	seedLegacyRow(t, db, "asset-passwords", "asset_passwords", "7", legacyVaultRow("7"))
	seedLegacyRow(t, db, "assets", "assets", "5", asset("5"))
	seedLegacyRow(t, db, "companies_assets", "", "6", asset("6"))
	if _, err := db.Exec(`PRAGMA user_version = 13`); err != nil {
		t.Fatalf("set user_version: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	s, err = Open(dbPath)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	db = s.DB()
	assertNoProtectedSentinel(t, db)

	var version int
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatalf("user_version: %v", err)
	}
	if version != 14 {
		t.Errorf("user_version = %d after the upgrade open, want 14", version)
	}
	var data string
	if err := db.QueryRow(`SELECT data FROM assets WHERE id = '5'`).Scan(&data); err != nil {
		t.Fatalf("select assets 5: %v", err)
	}
	if !strings.Contains(data, "dc01") {
		t.Fatalf("a Text field value the stored layout declares was not kept: %s", data)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	// No byte of a secret in any database file, the WAL included.
	matches, _ := filepath.Glob(dbPath + "*")
	for _, m := range matches {
		b, err := os.ReadFile(m)
		if err != nil {
			continue
		}
		for _, sentinel := range protectSentinels {
			if strings.Contains(string(b), sentinel) {
				t.Errorf("database file %s still holds %s after the upgrade scrub", filepath.Base(m), sentinel)
			}
		}
	}
}

// Assets (store funnel): an old asset row holds a password-type field value
// and the incoming row clears that field to "". The keep-richer merge keeps
// the old value for an empty incoming one; it must not survive.
func TestProtectFunnelScrubsClearedAssetPasswordField(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()
	db := s.DB()
	seedLegacyRow(t, db, "assets", "assets", "5", `{"id":5,"asset_layout_id":3,"fields":[{"id":10,"label":"Admin Password","value":"`+protectOldAF+`","position":1}]}`)

	if err := s.UpsertAssets(json.RawMessage(`{"id":5,"asset_layout_id":3,"fields":[{"id":10,"label":"Admin Password","value":"","position":1}]}`)); err != nil {
		t.Fatalf("UpsertAssets: %v", err)
	}
	assertNoProtectedSentinel(t, db)
}

// The read-only open (MCP `sql`, MCP `search`, local reads) of an upgraded
// store that no write has reopened yet: it must not read the plaintext rows.
func TestProtectReadOnlyOpenScrubsUpgradedStore(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "data.db")
	s, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	seedLegacyRow(t, s.DB(), "asset-passwords", "asset_passwords", "7", legacyVaultRow("7"))
	if _, err := s.DB().Exec(`PRAGMA user_version = 13`); err != nil {
		t.Fatalf("set user_version: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	ro, err := OpenReadOnly(dbPath)
	if err != nil {
		t.Fatalf("open read-only: %v", err)
	}
	defer ro.Close()
	var data string
	if err := ro.DB().QueryRow(`SELECT data FROM asset_passwords WHERE id='7'`).Scan(&data); err != nil {
		t.Fatalf("select: %v", err)
	}
	if strings.Contains(data, protectOldPW) || strings.Contains(data, protectOldOTP) {
		t.Errorf("a read-only open of an upgraded store returned the stored secret: %s", data)
	}
	assertNoProtectedSentinel(t, ro.DB())
}
