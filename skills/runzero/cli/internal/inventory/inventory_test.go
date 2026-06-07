// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package inventory

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// fakeGetter returns canned Export payloads keyed by path. Swap `payloads`
// between SyncAll calls to simulate inventory change across two runs.
type fakeGetter struct{ payloads map[string]string }

func (f *fakeGetter) Get(_ context.Context, path string, _ map[string]string) (json.RawMessage, error) {
	if p, ok := f.payloads[path]; ok {
		return json.RawMessage(p), nil
	}
	return json.RawMessage(`[]`), nil
}

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "inv.db")
	db, err := sql.Open("sqlite", dbPath+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := EnsureSchema(context.Background(), db); err != nil {
		t.Fatalf("schema: %v", err)
	}
	return db
}

// nowMinusDays returns a unix timestamp `days` in the past.
func nowMinusDays(days int) int64 {
	return time.Now().Add(-time.Duration(days) * 24 * time.Hour).Unix()
}

func seedRun1() map[string]string {
	recent := nowMinusDays(1)
	old := nowMinusDays(90)
	assets := `[
		{"id":"a1","names":["web01"],"addresses":["10.1.0.5"],"os":"Ubuntu Linux","type":"server","criticality":"critical","alive":true,"external":true,"last_seen":` + itoa(recent) + `,"tags":{"role":"web"},"owners":["dana"]},
		{"id":"a2","names":["db01"],"addresses":["10.1.0.6"],"os":"Windows Server 2012","type":"server","criticality":"high","alive":true,"external":false,"last_seen":` + itoa(old) + `,"os_eol":` + itoa(nowMinusDays(10)) + `,"tags":{}},
		{"id":"a3","names":["printer1"],"addresses":["192.168.9.20"],"os":"embedded","type":"printer","criticality":"low","alive":true,"last_seen":` + itoa(recent) + `}
	]`
	services := `[
		{"id":"s1","asset_id":"a1","address":"10.1.0.5","transport":"tcp","port":443,"protocol":"https","product":"nginx"},
		{"id":"s2","asset_id":"a1","address":"10.1.0.5","transport":"tcp","port":22,"protocol":"ssh"},
		{"id":"s3","asset_id":"a2","address":"10.1.0.6","transport":"tcp","port":445,"protocol":"smb2"}
	]`
	software := `[
		{"id":"sw1","asset_id":"a1","vendor":"OpenBSD","product":"OpenSSL","version":"3.0.2"},
		{"id":"sw2","asset_id":"a2","vendor":"OpenBSD","product":"OpenSSL","version":"1.0.1"},
		{"id":"sw3","asset_id":"a3","product":"OpenSSL","version":"3.0.2"}
	]`
	vulns := `[
		{"id":"v1","asset_id":"a1","name":"XZ backdoor","cve":"CVE-2024-3094","severity":"critical","cvss":10.0},
		{"id":"v2","asset_id":"a2","name":"SMB issue","cve":"CVE-2020-0796","severity":"high","cvss":8.1}
	]`
	return map[string]string{
		"/export/org/assets.json":          assets,
		"/export/org/services.json":        services,
		"/export/org/software.json":        software,
		"/export/org/vulnerabilities.json": vulns,
	}
}

func itoa(i int64) string { b, _ := json.Marshal(i); return string(b) }

func TestSyncAndTriage(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	g := &fakeGetter{payloads: seedRun1()}
	res, err := SyncAll(ctx, g, db, nil, "")
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if res.Counts["assets"] != 3 || res.Counts["services"] != 3 || res.Counts["vulnerabilities"] != 2 {
		t.Fatalf("unexpected counts: %+v", res.Counts)
	}

	rows, err := Triage(ctx, db, false)
	if err != nil {
		t.Fatalf("triage: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("want 3 triage rows, got %d", len(rows))
	}
	// a1 is critical with a critical vuln -> must rank first.
	if rows[0].AssetID != "a1" {
		t.Fatalf("want a1 first, got %s", rows[0].AssetID)
	}
	if rows[0].MaxSeverity != "critical" || rows[0].TopCVE != "CVE-2024-3094" {
		t.Fatalf("a1 triage wrong: %+v", rows[0])
	}
	if rows[0].ServiceCount != 2 {
		t.Fatalf("a1 want 2 services, got %d", rows[0].ServiceCount)
	}

	// internet-facing restricts to external assets (a1 only).
	ext, _ := Triage(ctx, db, true)
	if len(ext) != 1 || ext[0].AssetID != "a1" {
		t.Fatalf("internet-facing want [a1], got %+v", ext)
	}
}

func TestAffected(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	g := &fakeGetter{payloads: seedRun1()}
	if _, err := SyncAll(ctx, g, db, nil, ""); err != nil {
		t.Fatal(err)
	}
	// Case-insensitive substring match tolerant of the hyphens in a CVE id.
	rows, err := Affected(ctx, db, "cve-2024-3094")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].AssetID != "a1" {
		t.Fatalf("affected want [a1], got %+v", rows)
	}
	if len(rows[0].Services) == 0 {
		t.Fatalf("affected a1 should list its services, got none")
	}
	// Unknown CVE -> empty, no error.
	none, _ := Affected(ctx, db, "CVE-1999-0001")
	if len(none) != 0 {
		t.Fatalf("unknown CVE should be empty, got %+v", none)
	}
}

func TestSoftwareRollup(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	g := &fakeGetter{payloads: seedRun1()}
	if _, err := SyncAll(ctx, g, db, nil, ""); err != nil {
		t.Fatal(err)
	}
	rows, err := SoftwareRollup(ctx, db, "openssl")
	if err != nil {
		t.Fatal(err)
	}
	// 3.0.2 on a1+a3 (count 2), 1.0.1 on a2 (count 1). Most-deployed first.
	if len(rows) != 2 {
		t.Fatalf("want 2 version groups, got %d: %+v", len(rows), rows)
	}
	if rows[0].Version != "3.0.2" || rows[0].AssetCount != 2 {
		t.Fatalf("top rollup want 3.0.2 x2, got %+v", rows[0])
	}
}

func TestStale(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	g := &fakeGetter{payloads: seedRun1()}
	if _, err := SyncAll(ctx, g, db, nil, ""); err != nil {
		t.Fatal(err)
	}
	rows, err := Stale(ctx, db, 30)
	if err != nil {
		t.Fatal(err)
	}
	// a2 last seen 90d ago + EOL OS + untagged -> stale; a1/a3 seen recently.
	var foundA2 bool
	for _, r := range rows {
		if r.AssetID == "a2" {
			foundA2 = true
			if r.AgeDays < 30 {
				t.Fatalf("a2 age should be >30d, got %d", r.AgeDays)
			}
		}
		if r.AssetID == "a1" {
			t.Fatalf("a1 was seen recently and is tagged/owned; should not be stale: %+v", r)
		}
	}
	if !foundA2 {
		t.Fatalf("a2 should be stale, rows=%+v", rows)
	}
}

func TestExposureMap(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	g := &fakeGetter{payloads: seedRun1()}
	if _, err := SyncAll(ctx, g, db, nil, ""); err != nil {
		t.Fatal(err)
	}
	rows, err := ExposureMap(ctx, db, "10.0.0.0/8")
	if err != nil {
		t.Fatal(err)
	}
	// a1 (10.1.0.5): 443,22 ; a2 (10.1.0.6): 445. a3 (192.168.*) excluded.
	if len(rows) != 3 {
		t.Fatalf("want 3 exposed ports in 10/8, got %d: %+v", len(rows), rows)
	}
	for _, r := range rows {
		if r.AssetCount < 1 {
			t.Fatalf("exposure row should count >=1 asset: %+v", r)
		}
	}
	// Invalid CIDR is an actionable error.
	if _, err := ExposureMap(ctx, db, "not-a-cidr"); err == nil {
		t.Fatalf("expected error for invalid CIDR")
	}
}

func TestDiffAcrossRuns(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	// Run 1: a1,a2,a3.
	if _, err := SyncAll(ctx, &fakeGetter{payloads: seedRun1()}, db, nil, ""); err != nil {
		t.Fatal(err)
	}
	// Run 2: drop a3, add a4, keep a1/a2.
	run2 := seedRun1()
	run2["/export/org/assets.json"] = `[
		{"id":"a1","names":["web01"],"addresses":["10.1.0.5"],"criticality":"critical","last_seen":` + itoa(nowMinusDays(1)) + `},
		{"id":"a2","names":["db01"],"addresses":["10.1.0.6"],"criticality":"high","last_seen":` + itoa(nowMinusDays(1)) + `},
		{"id":"a4","names":["new-host"],"addresses":["10.1.0.9"],"criticality":"medium","last_seen":` + itoa(nowMinusDays(1)) + `}
	]`
	if _, err := SyncAll(ctx, &fakeGetter{payloads: run2}, db, nil, ""); err != nil {
		t.Fatal(err)
	}

	d, err := Diff(ctx, db, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !contains(d.Assets.Added, "new-host") {
		t.Fatalf("diff should report new-host added, got %+v", d.Assets.Added)
	}
	if !contains(d.Assets.Removed, "printer1") {
		t.Fatalf("diff should report printer1 removed, got %+v", d.Assets.Removed)
	}
}

func TestEmptyStoreDegradesGracefully(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	// Every query on a fresh, never-synced store must return empty + nil error.
	if r, err := Triage(ctx, db, false); err != nil || len(r) != 0 {
		t.Fatalf("triage empty: %v %v", r, err)
	}
	if r, err := Affected(ctx, db, "CVE-2024-3094"); err != nil || len(r) != 0 {
		t.Fatalf("affected empty: %v %v", r, err)
	}
	if r, err := SoftwareRollup(ctx, db, "openssl"); err != nil || len(r) != 0 {
		t.Fatalf("rollup empty: %v %v", r, err)
	}
	if r, err := Stale(ctx, db, 30); err != nil || len(r) != 0 {
		t.Fatalf("stale empty: %v %v", r, err)
	}
	if r, err := ExposureMap(ctx, db, "10.0.0.0/8"); err != nil || len(r) != 0 {
		t.Fatalf("exposure empty: %v %v", r, err)
	}
	d, err := Diff(ctx, db, 0)
	if err != nil || len(d.Assets.Added) != 0 || len(d.Assets.Removed) != 0 {
		t.Fatalf("diff empty: %+v %v", d, err)
	}
	st, err := Status(ctx, db)
	if err != nil || st.Tables["assets"] != 0 {
		t.Fatalf("status empty: %+v %v", st, err)
	}
}

// TestNullColumnsDoNotDropRows guards the COALESCE fix: an asset with NULL
// text columns (no OS, name, criticality — common for printers/unknown devices)
// must still appear in triage/stale/exposure, not be silently dropped by a
// Scan-into-string failure.
func TestNullColumnsDoNotDropRows(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	now := time.Now().Unix()
	old := nowMinusDays(90)
	if _, err := db.ExecContext(ctx, `INSERT INTO inv_sync_runs(id,started_at,finished_at) VALUES(1,?,?)`, now, now); err != nil {
		t.Fatal(err)
	}
	// NULL name/address/criticality/os/tags/owners on purpose.
	if _, err := db.ExecContext(ctx, `INSERT INTO inv_assets(id,last_seen,first_run,last_run) VALUES('n1',?,1,1)`, old); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO inv_services(id,asset_id,port,first_run,last_run) VALUES('ns1','n1',8080,1,1)`); err != nil {
		t.Fatal(err)
	}
	// Give n1 an address inside the test CIDR for the exposure check.
	if _, err := db.ExecContext(ctx, `UPDATE inv_assets SET address='10.2.3.4' WHERE id='n1'`); err != nil {
		t.Fatal(err)
	}
	if rows, _ := Triage(ctx, db, false); len(rows) != 1 || rows[0].AssetID != "n1" {
		t.Fatalf("triage dropped the NULL-column asset: %+v", rows)
	}
	if rows, _ := Stale(ctx, db, 30); len(rows) != 1 {
		t.Fatalf("stale dropped the NULL-column asset (90d old, untagged): %+v", rows)
	}
	if rows, _ := ExposureMap(ctx, db, "10.0.0.0/8"); len(rows) != 1 {
		t.Fatalf("exposure dropped the NULL-column service: %+v", rows)
	}
}

func contains(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}
