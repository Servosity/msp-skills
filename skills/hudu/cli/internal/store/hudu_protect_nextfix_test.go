// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-fix (handfixes.json vault-secrets-withheld): the store half of the
// extra-round classes. A read-only open that cannot prove the store was
// scrubbed refuses instead of reading; an unstamped legacy store is scrubbed
// before it is read; a legacy row of an unprotected type that holds a
// redirect's vault or asset answer is scrubbed on upgrade. These tests use
// only store APIs that predate the fix, so they also run (and fail) against
// the unfixed store.

package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

// seedLegacyStore writes a store the way an older binary left it: rows that
// hold plaintext secrets, stamped with version.
func seedLegacyStore(t *testing.T, version int, seed func(db *sql.DB)) string {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "data.db")
	s, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	seed(s.DB())
	if _, err := s.DB().Exec(`PRAGMA user_version = ` + strconv.Itoa(version)); err != nil {
		t.Fatalf("set user_version: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	return dbPath
}

// Codex extra P1 (read-only open): a concurrent writer can make the stamp
// read fail with SQLITE_BUSY between the read-only open and the check. Before
// the fix the check read any failure as "no scrub needed" and MCP `sql` got
// the unscrubbed store. A writer lock held from outside fails the open itself,
// so the race cannot be staged black-box; this pins the check's contract on a
// handle whose every query fails.
func TestProtectReadOnlyStampReadFailsClosed(t *testing.T) {
	dbPath := seedLegacyStore(t, 13, func(db *sql.DB) {
		seedLegacyRow(t, db, "asset-passwords", "asset_passwords", "7", legacyVaultRow("7"))
	})
	db, err := sql.Open("sqlite", "file:"+dbPath+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	needs, err := readOnlyStoreNeedsScrub(context.Background(), db)
	if err == nil && !needs {
		t.Fatal("a stamp that could not be read was taken as proof the store is scrubbed")
	}
}

// Unstamped legacy store: older binaries left an existing store at
// user_version 0. Before the fix a read-only open left it alone and MCP `sql`
// read the old password.
func TestProtectReadOnlyOpenScrubsUnstampedStore(t *testing.T) {
	dbPath := seedLegacyStore(t, 0, func(db *sql.DB) {
		seedLegacyRow(t, db, "asset-passwords", "asset_passwords", "7", legacyVaultRow("7"))
	})
	ro, err := OpenReadOnly(dbPath)
	if err != nil {
		t.Fatalf("open read-only: %v", err)
	}
	defer ro.Close()
	assertNoProtectedSentinel(t, ro.DB())
}

// Redirect, stored: an older binary that followed a redirect from an
// unprotected list into the vault or assets stored the answer under the
// unprotected type (its sync takes the one array in any envelope). With no
// answered URL on disk, the upgrade must scrub those rows too, and keep an
// ordinary row's content and its index entry.
func TestProtectUpgradeScrubsRedirectedLegacyRows(t *testing.T) {
	const company = `{"id":80,"name":"Acme","city":"Austin"}`
	assetBody := `{"id":5,"asset_layout_id":3,"fields":[{"label":"Admin Password","value":"` + protectOldAF + `"}]}`
	dbPath := seedLegacyStore(t, 13, func(db *sql.DB) {
		seedLegacyRow(t, db, "companies", "companies", "77", legacyVaultRow("77"))
		seedLegacyRow(t, db, "companies", "companies", "78", assetBody)
		seedLegacyRow(t, db, "companies", "companies", "80", company)
		// An index entry no stored row owns, left by a re-keyed legacy row.
		if _, err := db.Exec(`INSERT INTO resources_fts (rowid, id, resource_type, content) VALUES (?, ?, ?, ?)`, ftsRowID("companies", "99"), "99", "companies", legacyVaultRow("99")); err != nil {
			t.Fatalf("seed orphan index entry: %v", err)
		}
	})
	s, err := Open(dbPath)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer s.Close()
	assertNoProtectedSentinel(t, s.DB())
	var typed string
	if err := s.DB().QueryRow(`SELECT group_concat(data) FROM companies`).Scan(&typed); err != nil {
		t.Fatalf("select companies: %v", err)
	}
	for _, sentinel := range protectSentinels {
		if strings.Contains(typed, sentinel) {
			t.Errorf("the typed companies table still holds %s: %s", sentinel, typed)
		}
	}
	var kept string
	if err := s.DB().QueryRow(`SELECT data FROM resources WHERE resource_type = 'companies' AND id = '80'`).Scan(&kept); err != nil {
		t.Fatalf("select company 80: %v", err)
	}
	var want, got any
	if err := json.Unmarshal([]byte(company), &want); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(kept), &got); err != nil || !reflect.DeepEqual(want, got) {
		t.Errorf("an ordinary row lost content: %s", kept)
	}
	var own int
	if err := s.DB().QueryRow(`SELECT count(*) FROM resources_fts WHERE rowid = ?`, ftsRowID("companies", "80")).Scan(&own); err != nil || own != 1 {
		t.Errorf("an ordinary row lost its search index entry: count=%d err=%v", own, err)
	}
}
