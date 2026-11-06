// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package inventory

// Codex round 1 of the runzero 4.34.2 reprint review: the offline novels read
// the local inventory as runZero's current attack surface. Every fixture row
// is shaped as runZero's OpenAPI documents its export rows (Asset; Service,
// Software and Vulnerability rows embed their asset, keyed by
// <prefix>_id and linked by <prefix>_asset_id; Certificate.validity_end).

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"
)

func r1Asset(id, name, addr, os string) string {
	return `{"id":"` + id + `","names":["` + name + `"],"addresses":["` + addr + `"],"os":"` + os +
		`","criticality_rank":4,"alive":true,"last_seen":` + itoa(nowMinusDays(1)) + `,"tags":{"role":"x"},"owners":["dana"]}`
}

func r1Service(assetID, addr, svcID string, port int, proto string) string {
	return `{"id":"` + assetID + `","service_id":"` + svcID + `","service_asset_id":"` + assetID + `","service_address":"` + addr +
		`","service_transport":"tcp","service_port":` + itoa(int64(port)) + `,"service_protocol":"` + proto + `"}`
}

func r1Vuln(assetID, vulnID, cve, sev string) string {
	return `{"id":"` + assetID + `","vulnerability_id":"` + vulnID + `","vulnerability_asset_id":"` + assetID +
		`","vulnerability_name":"` + cve + `","vulnerability_cve":"` + cve + `","vulnerability_severity":"` + sev + `"}`
}

func r1Software(assetID, swID, product, version string) string {
	return `{"software_id":"` + swID + `","software_asset_id":"` + assetID + `","software_vendor":"OpenBSD","software_product":"` +
		product + `","software_version":"` + version + `"}`
}

func r1Cert(id string, end int64) string {
	return `{"id":"` + id + `","subject":"CN=web01","issuer":"CN=R3","validity_end":` + itoa(end) + `}`
}

func r1Sync(t *testing.T, db *sql.DB, payloads map[string]string, only []string, search string) *SyncResult {
	t.Helper()
	res, err := SyncAll(context.Background(), &fakeGetter{payloads: payloads}, db, only, search)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

// Class historical-inventory-read-as-current (Codex r1 [P1] "historical
// inventory treated as current" and "empty refresh read as old inventory"):
// a row runZero stopped returning stays in the store (its last_run is older),
// and every novel read it as current: `affected` listed the remediated
// finding, triage counted it and the removed service, the software rollup and
// exposure map kept removed rows, and a refresh that returned no rows advanced
// no row's last_run, so the "latest" rows were the previous snapshot. Every
// query reads each entity's current snapshot: the rows of the latest run that
// refreshed that entity.
func TestR1NovelsReadOnlyTheCurrentSnapshot(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	soon := clock().Add(10 * 24 * time.Hour).Unix()
	run1 := map[string]string{
		"/export/org/assets.json": `[` + r1Asset("a1", "web01", "10.0.0.5", "Ubuntu") + `]`,
		"/export/org/services.json": `[` + r1Service("a1", "10.0.0.5", "svc-22", 22, "ssh") + `,` +
			r1Service("a1", "10.0.0.5", "svc-443", 443, "tls") + `]`,
		"/export/org/software.json": `[` + r1Software("a1", "sw-1", "OpenSSH", "9.6") + `]`,
		"/export/org/vulnerabilities.json": `[` + r1Vuln("a1", "v-1", "CVE-2024-6387", "Critical") + `,` +
			r1Vuln("a1", "v-2", "CVE-2023-48795", "Medium") + `]`,
		"/export/org/certificates.json": `[` + r1Cert("c1", soon) + `]`,
	}
	r1Sync(t, db, run1, nil, "")
	// Run 2: the regreSSHion finding is remediated, the SSH service and the
	// OpenSSH package are gone, the certificate was replaced (none returned).
	run2 := map[string]string{
		"/export/org/assets.json":          run1["/export/org/assets.json"],
		"/export/org/services.json":        `[` + r1Service("a1", "10.0.0.5", "svc-443", 443, "tls") + `]`,
		"/export/org/software.json":        `[]`,
		"/export/org/vulnerabilities.json": `[` + r1Vuln("a1", "v-2", "CVE-2023-48795", "Medium") + `]`,
		"/export/org/certificates.json":    `[]`,
	}
	r1Sync(t, db, run2, nil, "")

	if rows, _ := Affected(ctx, db, "CVE-2024-6387"); len(rows) != 0 {
		t.Errorf("affected lists the remediated finding: %+v", rows)
	}
	if rows, _ := Affected(ctx, db, "CVE-2023-48795"); len(rows) != 1 || strings.Join(rows[0].Services, ",") != "tls/443" {
		t.Errorf("affected CVE-2023-48795 = %+v, want a1 with only tls/443", rows)
	}
	tri, _ := Triage(ctx, db, false)
	if len(tri) != 1 || tri[0].VulnCount != 1 || tri[0].ServiceCount != 1 || tri[0].TopCVE != "CVE-2023-48795" || tri[0].MaxSeverity != "medium" {
		t.Errorf("triage = %+v, want a1 with 1 finding (CVE-2023-48795, medium) and 1 service", tri)
	}
	if rows, _ := SoftwareRollup(ctx, db, "openssh"); len(rows) != 0 {
		t.Errorf("software rollup lists the removed package: %+v", rows)
	}
	if rows, _ := ExposureMap(ctx, db, "10.0.0.0/24"); len(rows) != 1 || rows[0].Port != 443 {
		t.Errorf("exposure map = %+v, want only 443", rows)
	}
	if rows, _ := CertsExpiring(ctx, db, 30, false); len(rows) != 0 {
		t.Errorf("certs-expiring lists a certificate the latest refresh did not return: %+v", rows)
	}
	if rows, _ := List(ctx, db, "vulnerabilities", 50, ""); len(rows) != 1 || !strings.Contains(string(rows[0]), "CVE-2023-48795") {
		t.Errorf("inventory list vulnerabilities = %s, want only v-2", rows)
	}
	st, _ := Status(ctx, db)
	if st.Tables["vulnerabilities"] != 1 || st.Tables["software"] != 0 || st.StoredRows["vulnerabilities"] != 2 {
		t.Errorf("status = %+v, want 1 current finding (2 stored) and no current software", st)
	}

	// Codex's input: `inventory sync --only assets` returns one asset, then
	// the asset is deleted and the export returns [].
	db2 := openTestDB(t)
	r1Sync(t, db2, map[string]string{"/export/org/assets.json": `[` + r1Asset("a9", "gone01", "10.0.0.9", "Windows") + `]`}, []string{"assets"}, "")
	r1Sync(t, db2, map[string]string{"/export/org/assets.json": `[]`}, []string{"assets"}, "")
	if rows, _ := List(ctx, db2, "assets", 50, ""); len(rows) != 0 {
		t.Errorf("inventory list assets returns the deleted asset: %s", rows)
	}
	if rows, _ := Triage(ctx, db2, false); len(rows) != 0 {
		t.Errorf("triage ranks the deleted asset: %+v", rows)
	}
	if rows, _ := Stale(ctx, db2, 30); len(rows) != 0 {
		t.Errorf("stale lists the deleted asset: %+v", rows)
	}
	if st, _ := Status(ctx, db2); st.Tables["assets"] != 0 {
		t.Errorf("status counts the deleted asset as current: %+v", st)
	}
}

// Class snapshot-gap-read-as-continuous-presence (Codex r1 [P1]): a row's
// first_run is the run it was first ever seen, so a row that was absent from
// a refresh and came back was never "added". Presence is recorded per refresh
// (inv_presence: each row's unbroken streaks of the refreshes that returned
// it), and diff and exposure-delta compare presence at the two runs.
func TestR1ReturningRowsAreAdded(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	asset := map[string]string{"/export/org/assets.json": `[` + r1Asset("a1", "web01", "10.0.0.5", "Ubuntu") + `]`}
	none := map[string]string{"/export/org/assets.json": `[]`}
	// Codex's input: alive, offline, alive again (--search alive:true).
	r1Sync(t, db, asset, []string{"assets"}, "alive:true")
	r1Sync(t, db, none, []string{"assets"}, "alive:true")
	r1Sync(t, db, asset, []string{"assets"}, "alive:true")
	d, _ := Diff(ctx, db, 0)
	if !contains(d.Assets.Added, "web01") || len(d.Assets.Removed) != 0 {
		t.Errorf("diff after the asset came back = %+v, want web01 added", d.Assets)
	}
	r1Sync(t, db, none, []string{"assets"}, "alive:true")
	if d, _ := Diff(ctx, db, 0); !contains(d.Assets.Removed, "web01") || len(d.Assets.Added) != 0 {
		t.Errorf("diff after it left again = %+v, want web01 removed", d.Assets)
	}

	// exposure-delta: a service that was closed and reopened is newly exposed.
	svc := map[string]string{"/export/org/services.json": `[` + r1Service("a1", "10.0.0.5", "svc-3389", 3389, "rdp") + `]`}
	closed := map[string]string{"/export/org/services.json": `[]`}
	db2 := openTestDB(t)
	r1Sync(t, db2, svc, []string{"services"}, "")
	r1Sync(t, db2, closed, []string{"services"}, "")
	r1Sync(t, db2, svc, []string{"services"}, "")
	res, _ := ExposureDelta(ctx, db2, 0)
	if len(res.NewServices) != 1 || res.NewServices[0].Port != 3389 {
		t.Errorf("exposure-delta = %+v, want the reopened RDP service", res.NewServices)
	}
}

// Class exposure-counted-by-address (Codex r1 [P1] wrong asset count): the
// exposure map counted distinct addresses, so two runZero assets sharing an
// address (separate sites, NAT) counted once. It counts service_asset_id.
func TestR1ExposureMapCountsAssets(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	r1Sync(t, db, map[string]string{"/export/org/services.json": `[` + r1Service("a1", "10.0.0.5", "svc-1", 443, "tls") + `,` +
		r1Service("a2", "10.0.0.5", "svc-2", 443, "tls") + `]`}, []string{"services"}, "")
	rows, _ := ExposureMap(ctx, db, "10.0.0.0/24")
	if len(rows) != 1 || rows[0].AssetCount != 2 {
		t.Errorf("exposure map = %+v, want tcp/443 on 2 assets", rows)
	}
}

// Class incomplete-inventory-read-as-complete (Codex r1 [P1]): a sync whose
// vulnerabilities export runZero refused (the spec's documented 403) left
// triage answering vuln_count 0 for every asset with nothing to say the
// findings were never fetched. Coverage names every entity a result reads
// that was never synced or whose latest sync failed.
func TestR1CoverageNamesMissingAndFailedEntities(t *testing.T) {
	ctx := context.Background()
	need := []string{"assets", "services", "vulnerabilities"}
	full := map[string]string{
		"/export/org/assets.json":          `[` + r1Asset("a1", "web01", "10.0.0.5", "Ubuntu") + `]`,
		"/export/org/vulnerabilities.json": `[` + r1Vuln("a1", "v-1", "CVE-2024-6387", "Critical") + `]`,
	}
	refused := &failingGetter{fakeGetter: fakeGetter{payloads: full}, refuse: map[string]bool{"/export/org/vulnerabilities.json": true}}

	db := openTestDB(t)
	if _, err := SyncAll(ctx, refused, db, nil, ""); err != nil {
		t.Fatal(err)
	}
	got := strings.Join(Coverage(ctx, db, need...), "; ")
	if !strings.Contains(got, "vulnerabilities") || !strings.Contains(got, "403") {
		t.Errorf("coverage after a refused vulnerabilities export = %q, want vulnerabilities named with the error", got)
	}
	if strings.Contains(got, "assets") || strings.Contains(got, "services") {
		t.Errorf("coverage names an entity that synced: %q", got)
	}

	db2 := openTestDB(t)
	r1Sync(t, db2, full, nil, "")
	if got := Coverage(ctx, db2, need...); len(got) != 0 {
		t.Errorf("coverage after a full sync = %q, want none", got)
	}
	r1Sync(t, db2, full, []string{"assets"}, "") // skipped by the operator: not a failure
	if got := Coverage(ctx, db2, need...); len(got) != 0 {
		t.Errorf("coverage after --only assets = %q, want none", got)
	}
	if _, err := SyncAll(ctx, refused, db2, nil, ""); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(Coverage(ctx, db2, need...), "; "); !strings.Contains(got, "vulnerabilities") {
		t.Errorf("coverage after the latest vulnerabilities sync failed = %q, want it named", got)
	}
	db3 := openTestDB(t)
	r1Sync(t, db3, full, []string{"assets"}, "")
	if got := strings.Join(Coverage(ctx, db3, need...), "; "); !strings.Contains(got, "vulnerabilities") || !strings.Contains(got, "services") {
		t.Errorf("coverage with services and vulnerabilities never synced = %q", got)
	}
}

// Class filtered-snapshot-read-as-complete (Codex r1 [P1]): diff compared a
// run filtered by --search with an unfiltered one, so every asset the filter
// left out read as removed. A comparison takes its baseline from the runs
// with the same search scope; an entity with none is named in
// no_baseline_entities, and a result joining entities synced under different
// scopes is named by Coverage.
func TestR1ComparisonsUseTheSameScope(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	linux := r1Asset("a1", "linux01", "10.0.0.5", "Ubuntu")
	windows := r1Asset("a2", "win01", "10.0.0.6", "Windows")
	r1Sync(t, db, map[string]string{"/export/org/assets.json": `[` + linux + `,` + windows + `]`}, []string{"assets"}, "")
	r1Sync(t, db, map[string]string{"/export/org/assets.json": `[` + windows + `]`}, []string{"assets"}, "os:Windows")
	d, _ := Diff(ctx, db, 0)
	if len(d.Assets.Removed) != 0 || len(d.Assets.Added) != 0 {
		t.Errorf("diff across a filtered and an unfiltered run = %+v, want no change", d.Assets)
	}
	if !contains(d.NoBaselineEntities, "assets") {
		t.Errorf("diff does not say assets had no same-scope baseline: %+v", d)
	}
	r1Sync(t, db, map[string]string{"/export/org/assets.json": `[]`}, []string{"assets"}, "os:Windows")
	if d, _ := Diff(ctx, db, 0); !contains(d.Assets.Removed, "win01") || contains(d.Assets.Removed, "linux01") {
		t.Errorf("diff of two os:Windows runs = %+v, want only win01 removed", d.Assets)
	}
	if got := FilteredScope(ctx, db, "assets"); got != "os:Windows" {
		t.Errorf("FilteredScope(assets) = %q, want os:Windows (a result over it must say it is filtered)", got)
	}

	// exposure-delta: the baseline for a filtered services run is the
	// previous run of the same filter.
	db2 := openTestDB(t)
	a := r1Service("a1", "10.0.0.5", "svc-a", 22, "ssh")
	b := r1Service("a2", "10.0.0.6", "svc-b", 3389, "rdp")
	c := r1Service("a2", "10.0.0.6", "svc-c", 445, "smb")
	r1Sync(t, db2, map[string]string{"/export/org/services.json": `[` + a + `,` + b + `]`}, []string{"services"}, "")
	r1Sync(t, db2, map[string]string{"/export/org/services.json": `[` + b + `]`}, []string{"services"}, "os:Windows")
	r1Sync(t, db2, map[string]string{"/export/org/services.json": `[` + b + `,` + c + `]`}, []string{"services"}, "os:Windows")
	res, _ := ExposureDelta(ctx, db2, 0)
	if len(res.NewServices) != 1 || res.NewServices[0].Port != 445 {
		t.Errorf("exposure-delta of two os:Windows runs = %+v, want only svc-c", res.NewServices)
	}

	// A result joining assets synced unfiltered and findings synced under a
	// filter reads two different scopes.
	db3 := openTestDB(t)
	r1Sync(t, db3, map[string]string{"/export/org/assets.json": `[` + linux + `]`}, []string{"assets"}, "")
	r1Sync(t, db3, map[string]string{"/export/org/vulnerabilities.json": `[]`}, []string{"vulnerabilities"}, "severity:critical")
	if got := strings.Join(Coverage(ctx, db3, "assets", "vulnerabilities"), "; "); !strings.Contains(got, "severity:critical") {
		t.Errorf("coverage across mixed search scopes = %q, want the scopes named", got)
	}
}
