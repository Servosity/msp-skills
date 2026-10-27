// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-fix (handfixes.json vault-secrets-withheld): the store half of the
// next-batch review classes. The upgrade writes back the tree it inspected;
// an entry's own plain field_type never overrides the stored layout; a row the
// upgrade cannot key by id is scrubbed by rowid. These tests use only store
// APIs that predate the fix, so they also run (and fail) against the unfixed
// store.

package store

import (
	"database/sql"
	"encoding/json"
	"strings"
	"testing"
)

func assertNoSentinelIn(t *testing.T, db *sql.DB, table string) {
	t.Helper()
	var all sql.NullString
	if err := db.QueryRow(`SELECT group_concat(data) FROM "` + table + `"`).Scan(&all); err != nil {
		t.Fatalf("select %s: %v", table, err)
	}
	for _, sentinel := range protectSentinels {
		if strings.Contains(all.String, sentinel) {
			t.Errorf("%s still holds %s: %s", table, sentinel, all.String)
		}
	}
}

// Duplicate members: a stored row whose secret sits in a member the decoder
// drops (a duplicate "fields") was kept byte for byte, because the scrub found
// nothing to change and returned the stored bytes.
func TestProtectUpgradeWritesTheInspectedTree(t *testing.T) {
	dup := func(id string) string {
		return `{"id":` + id + `,"asset_layout_id":3,"fields":[{"label":"Admin Password","value":"` + protectOldAF + `"}],"fields":[]}`
	}
	dbPath := seedLegacyStore(t, 13, func(db *sql.DB) {
		seedLegacyRow(t, db, "assets", "assets", "5", dup("5"))
		seedLegacyRow(t, db, "companies", "companies", "6", dup("6"))
	})
	s, err := Open(dbPath)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer s.Close()
	assertNoProtectedSentinel(t, s.DB())
	assertNoSentinelIn(t, s.DB(), "companies")
}

// Store entry type: an asset entry whose own field_type says Text while the
// stored layout says Password kept its value, on upgrade and through the
// write funnel. MCP `sql` then read it.
func TestProtectStoreEntryTypeNeverOverridesLayout(t *testing.T) {
	const layout = `{"id":3,"name":"Server","fields":[{"label":"Admin Password","field_type":"Password"}]}`
	asset := func(id string) string {
		return `{"id":` + id + `,"asset_layout_id":3,"fields":[{"label":"Admin Password","field_type":"Text","value":"` + protectOldAF + `"}]}`
	}
	dbPath := seedLegacyStore(t, 13, func(db *sql.DB) {
		seedLegacyRow(t, db, "asset-layouts", "asset_layouts", "3", layout)
		seedLegacyRow(t, db, "assets", "assets", "5", asset("5"))
	})
	s, err := Open(dbPath)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer s.Close()
	assertNoProtectedSentinel(t, s.DB())
	if err := s.Upsert("assets", "6", json.RawMessage(asset("6"))); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	assertNoProtectedSentinel(t, s.DB())
}

// Rows without an id: a legacy typed table allows a NULL id, and the upgrade
// skipped every row it could not key, so MCP `sql` read the password.
func TestProtectUpgradeScrubsRowsWithoutAnID(t *testing.T) {
	assetRow := `{"id":5,"asset_layout_id":3,"fields":[{"label":"Admin Password","value":"` + protectOldAF + `"}]}`
	dbPath := seedLegacyStore(t, 13, func(db *sql.DB) {
		for table, data := range map[string]string{"asset_passwords": legacyVaultRow("7"), "assets": assetRow, "companies": legacyVaultRow("8")} {
			if _, err := db.Exec(`INSERT INTO "`+table+`" (id, data) VALUES (NULL, ?)`, data); err != nil {
				t.Fatalf("seed %s: %v", table, err)
			}
		}
	})
	s, err := Open(dbPath)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer s.Close()
	for _, table := range []string{"asset_passwords", "assets", "companies"} {
		assertNoSentinelIn(t, s.DB(), table)
	}
}
