// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package inventory

// Codex round 2 of the runzero 4.34.2 reprint review (.review/codex-r2.md).
// Fixture rows are shaped as runZero's OpenAPI documents its export rows (see
// r1_review_test.go).

import (
	"context"
	"database/sql"
	"encoding/json"
	"runtime"
	"strings"
	"testing"
	"time"
)

// r2ServiceNoAddr is a service row with no service_address: a result can
// place it only through its asset's address.
func r2ServiceNoAddr(assetID, svcID string, port int, proto string) string {
	return `{"id":"` + assetID + `","service_id":"` + svcID + `","service_asset_id":"` + assetID +
		`","service_transport":"tcp","service_port":` + itoa(int64(port)) + `,"service_protocol":"` + proto + `"}`
}

func r2Cert(id, assetID string, end int64) string {
	return `{"id":"` + id + `","asset_id":"` + assetID + `","subject":"CN=web01","issuer":"CN=R3","validity_end":` + itoa(end) + `}`
}

// Class historical-inventory-treated-as-current (Codex r2 [P1], repeat of
// r1-current-snapshot-per-entity): each child entity was read at its current
// snapshot, but the asset side of its join read any stored asset row, so a
// finding, service or certificate of an asset runZero deleted showed the
// asset's old name, address and criticality as current, and nothing said the
// asset was gone. Every join reads assets at their current snapshot, and
// Coverage names each current row whose asset that snapshot does not hold.
func TestR2JoinsReadAssetsAtTheCurrentSnapshot(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	soon := clock().Add(10 * 24 * time.Hour).Unix()
	asset := `[` + r1Asset("a1", "web01", "10.0.0.5", "Ubuntu") + `]`
	r1Sync(t, db, map[string]string{"/export/org/assets.json": asset}, nil, "")
	full := map[string]string{
		"/export/org/assets.json": asset,
		"/export/org/services.json": `[` + r1Service("a1", "10.0.0.5", "svc-22", 22, "ssh") + `,` +
			r2ServiceNoAddr("a1", "svc-80", 80, "http") + `]`,
		"/export/org/vulnerabilities.json": `[` + r1Vuln("a1", "v-1", "CVE-2024-6387", "Critical") + `]`,
		"/export/org/certificates.json":    `[` + r2Cert("c1", "a1", soon) + `]`,
	}
	r1Sync(t, db, full, nil, "")
	// Codex's input: asset A is deleted, then `inventory sync --only assets`
	// receives [] from /export/org/assets.json.
	r1Sync(t, db, map[string]string{"/export/org/assets.json": `[]`}, []string{"assets"}, "")

	rows, _ := Affected(ctx, db, "CVE-2024-6387")
	if len(rows) != 1 || rows[0].AssetID != "a1" || rows[0].Name != "" || rows[0].Address != "" || rows[0].Criticality != "" {
		t.Errorf("affected = %+v, want the finding listed by asset id a1 alone (no name, address or criticality of the deleted asset)", rows)
	}
	if rows, _ := ExposureMap(ctx, db, "10.0.0.0/24"); len(rows) != 1 || rows[0].Port != 22 {
		t.Errorf("exposure map = %+v, want only svc-22 (its own address); svc-80 has none but the deleted asset's", rows)
	}
	delta, _ := ExposureDelta(ctx, db, 0)
	if len(delta.NewServices) != 2 || len(delta.NewVulns) != 1 {
		t.Fatalf("exposure-delta = %+v, want svc-22, svc-80 and v-1 new", delta)
	}
	for _, s := range delta.NewServices {
		if s.AssetName != "" || s.Criticality != "" {
			t.Errorf("exposure-delta service %+v carries the deleted asset's attributes", s)
		}
	}
	if v := delta.NewVulns[0]; v.AssetName != "" || v.Address != "" || v.Criticality != "" {
		t.Errorf("exposure-delta finding %+v carries the deleted asset's attributes", v)
	}
	if certs, _ := CertsExpiring(ctx, db, 30, false); len(certs) != 1 || certs[0].AssetName != "" || certs[0].Address != "" || certs[0].Criticality != "" {
		t.Errorf("certs-expiring = %+v, want c1 without the deleted asset's attributes", certs)
	}
	got := strings.Join(Coverage(ctx, db, "vulnerabilities", "assets", "services"), "; ")
	for _, want := range []string{"v-1", "svc-22", "svc-80", "current asset snapshot"} {
		if !strings.Contains(got, want) {
			t.Errorf("coverage for affected = %q, want %q named", got, want)
		}
	}
	if got := strings.Join(Coverage(ctx, db, "certificates", "assets"), "; "); !strings.Contains(got, "c1") {
		t.Errorf("coverage for certs-expiring = %q, want c1 named", got)
	}

	// The other direction: with the asset back in the snapshot the joins
	// read it and Coverage names nothing.
	r1Sync(t, db, full, nil, "")
	if rows, _ := Affected(ctx, db, "CVE-2024-6387"); len(rows) != 1 || rows[0].Name != "web01" || rows[0].Address != "10.0.0.5" {
		t.Errorf("affected after the asset came back = %+v, want web01 at 10.0.0.5", rows)
	}
	if got := Coverage(ctx, db, "vulnerabilities", "assets", "services", "certificates"); len(got) != 0 {
		t.Errorf("coverage over a consistent store = %q, want none", got)
	}
}

// stopGetter answers like fakeGetter but never returns from the export at
// stopAt: it ends the syncing goroutine inside the fetch (runtime.Goexit), so
// nothing SyncAll would do after that export returns ever runs. The store is
// left as a sync killed before the export returned leaves it.
type stopGetter struct {
	fakeGetter
	stopAt string
}

func (g *stopGetter) Get(ctx context.Context, path string, params map[string]string) (json.RawMessage, error) {
	if path == g.stopAt {
		runtime.Goexit()
	}
	return g.fakeGetter.Get(ctx, path, params)
}

func killedSync(t *testing.T, db *sql.DB, g Getter, only []string) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = SyncAll(context.Background(), g, db, only, "")
		t.Error("the sync returned; the stopped export should have ended it")
	}()
	<-done
}

// Class incomplete-fetch-read-as-complete (Codex r2 [P1], repeat of
// r1-incomplete-inventory-partial-failure): a sync killed before an export
// returned recorded neither a refresh nor a failure, so the previous snapshot
// read as complete (affected: exit 0, no warning). Every entity a sync will
// refresh is recorded as not finished before any fetch, and Coverage names
// each one the sync did not refresh.
func TestR2KilledSyncLeavesItsEntitiesIncomplete(t *testing.T) {
	ctx := context.Background()
	full := map[string]string{
		"/export/org/assets.json":          `[` + r1Asset("a1", "web01", "10.0.0.5", "Ubuntu") + `]`,
		"/export/org/vulnerabilities.json": `[` + r1Vuln("a1", "v-1", "CVE-2024-6387", "Critical") + `]`,
	}
	db := openTestDB(t)
	r1Sync(t, db, full, nil, "")
	// Codex's input: `inventory sync --only vulnerabilities` killed while
	// /export/org/vulnerabilities.json has not answered.
	killedSync(t, db, &stopGetter{fakeGetter: fakeGetter{payloads: full}, stopAt: "/export/org/vulnerabilities.json"}, []string{"vulnerabilities"})
	got := strings.Join(Coverage(ctx, db, "vulnerabilities", "assets", "services"), "; ")
	if !strings.Contains(got, "sync of vulnerabilities") || !strings.Contains(got, "did not finish") {
		t.Errorf("coverage after a killed vulnerabilities sync = %q, want vulnerabilities named as not finished", got)
	}
	if got := Coverage(ctx, db, "assets", "services"); len(got) != 0 {
		t.Errorf("coverage names entities the killed sync was not asked to refresh: %q", got)
	}
	if st, _ := Status(ctx, db); !strings.Contains(strings.Join(st.Incomplete, "; "), "sync of vulnerabilities") {
		t.Errorf("inventory status incomplete = %q, want vulnerabilities", st.Incomplete)
	}
	// A full sync killed at its first export: every entity it was asked to
	// refresh and never reached is named.
	killedSync(t, db, &stopGetter{fakeGetter: fakeGetter{payloads: full}, stopAt: "/export/org/assets.json"}, nil)
	got = strings.Join(Coverage(ctx, db, Entities...), "; ")
	for _, e := range Entities {
		if !strings.Contains(got, "sync of "+e+" ") {
			t.Errorf("coverage after a full sync killed at its first export = %q, want %s named", got, e)
		}
	}

	// A store an older build wrote: its unfinished run recorded nothing, so
	// what it was asked to refresh is unknown; it counts for every entity.
	db2 := openTestDB(t)
	r1Sync(t, db2, full, nil, "")
	if _, err := db2.ExecContext(ctx, `INSERT INTO inv_sync_runs(started_at, note) VALUES(?, '')`, clock().Unix()); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(Coverage(ctx, db2, "assets"), "; "); !strings.Contains(got, "did not finish") {
		t.Errorf("coverage with an older build's unfinished run = %q, want assets named as not finished", got)
	}

	// The other direction: a sync that finishes names nothing.
	r1Sync(t, db, full, nil, "")
	r1Sync(t, db2, full, nil, "")
	for i, d := range []*sql.DB{db, db2} {
		if got := Coverage(ctx, d, Entities...); len(got) != 0 {
			t.Errorf("store %d: coverage after a sync that finished = %q, want none", i+1, got)
		}
	}
}

// forgetPresence leaves db as the ac30625 hand layer left its runs
// (inv_sync_entities recorded, no presence history) after the upgraded build
// opens it: EnsureSchema creates the presence tables empty.
func forgetPresence(t *testing.T, db *sql.DB) {
	t.Helper()
	ctx := context.Background()
	for _, q := range []string{`DROP TABLE IF EXISTS inv_presence`, `DROP TABLE IF EXISTS inv_presence_runs`} {
		if _, err := db.ExecContext(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	if err := EnsureSchema(ctx, db); err != nil {
		t.Fatal(err)
	}
}

func sameSet(got []string, want ...string) bool {
	if len(got) != len(want) {
		return false
	}
	for _, w := range want {
		if !contains(got, w) {
			return false
		}
	}
	return true
}

// Class missing-history-read-as-absence (Codex r2 [P1]): diff and
// exposure-delta compared presence at the baseline run whenever
// inv_sync_entities existed, but a store the ac30625 layer wrote has those
// rows and no presence history, so every unchanged asset, service and
// finding read as added and every removal since the baseline was dropped.
// Each refresh records that it wrote presence (inv_presence_runs), and a
// baseline without it is compared by first_run/last_run.
func TestR2BaselineWithoutPresenceHistory(t *testing.T) {
	ctx := context.Background()
	assets := func(rows ...string) string { return `[` + strings.Join(rows, ",") + `]` }
	a1 := r1Asset("a1", "web01", "10.0.0.5", "Ubuntu")
	a2 := r1Asset("a2", "db01", "10.0.0.6", "Windows")
	a3 := r1Asset("a3", "old01", "10.0.0.7", "Windows")
	a4 := r1Asset("a4", "new01", "10.0.0.8", "Ubuntu")
	svc22 := r1Service("a1", "10.0.0.5", "svc-22", 22, "ssh")
	svc445 := r1Service("a2", "10.0.0.6", "svc-445", 445, "smb")
	svc3389 := r1Service("a1", "10.0.0.5", "svc-3389", 3389, "rdp")
	sw1 := r1Software("a1", "sw-1", "OpenSSH", "9.6")
	sw2 := r1Software("a2", "sw-2", "Samba", "4.1")
	v1 := r1Vuln("a1", "v-1", "CVE-2023-48795", "Medium")
	v2 := r1Vuln("a1", "v-2", "CVE-2024-6387", "Critical")
	older := func(assetRows string) map[string]string {
		return map[string]string{
			"/export/org/assets.json":          assetRows,
			"/export/org/services.json":        assets(svc22, svc445),
			"/export/org/software.json":        assets(sw1, sw2),
			"/export/org/vulnerabilities.json": assets(v1),
		}
	}
	db := openTestDB(t)
	// Codex's input: two syncs by the older layer (old01 removed between
	// them), then the upgrade.
	r1Sync(t, db, older(assets(a1, a2, a3)), nil, "")
	r1Sync(t, db, older(assets(a1, a2)), nil, "")
	forgetPresence(t, db)

	d, _ := Diff(ctx, db, 0)
	if len(d.Assets.Added) != 0 || !sameSet(d.Assets.Removed, "old01") || len(d.Services.Added) != 0 || len(d.Software.Added) != 0 {
		t.Errorf("diff right after the upgrade = %+v, want only old01 removed", d)
	}
	if res, _ := ExposureDelta(ctx, db, 0); len(res.NewServices) != 0 || len(res.NewVulns) != 0 {
		t.Errorf("exposure-delta right after the upgrade = %+v %+v, want nothing new", res.NewServices, res.NewVulns)
	}

	// The upgraded build's first sync, compared with the older layer's run.
	now := map[string]string{
		"/export/org/assets.json":          assets(a1, a2, a4),
		"/export/org/services.json":        assets(svc22, svc3389),
		"/export/org/software.json":        assets(sw1),
		"/export/org/vulnerabilities.json": assets(v1, v2),
	}
	r1Sync(t, db, now, nil, "")
	d, _ = Diff(ctx, db, 0)
	if !sameSet(d.Assets.Added, "new01") || len(d.Assets.Removed) != 0 {
		t.Errorf("diff assets after the first upgraded sync = %+v, want only new01 added", d.Assets)
	}
	if !sameSet(d.Services.Added, "a1 rdp/3389") || !sameSet(d.Services.Removed, "a2 smb/445") {
		t.Errorf("diff services after the first upgraded sync = %+v, want rdp/3389 added and smb/445 removed", d.Services)
	}
	if len(d.Software.Added) != 0 || !sameSet(d.Software.Removed, "Samba 4.1") {
		t.Errorf("diff software after the first upgraded sync = %+v, want Samba 4.1 removed", d.Software)
	}
	res, _ := ExposureDelta(ctx, db, 0)
	if len(res.NewServices) != 1 || res.NewServices[0].Port != 3389 || len(res.NewVulns) != 1 || res.NewVulns[0].CVE != "CVE-2024-6387" {
		t.Errorf("exposure-delta after the first upgraded sync = %+v %+v, want only rdp/3389 and CVE-2024-6387", res.NewServices, res.NewVulns)
	}

	// From here the baseline has presence history: an identical sync
	// changes nothing.
	r1Sync(t, db, now, nil, "")
	d, _ = Diff(ctx, db, 0)
	for name, sec := range map[string]DiffSection{"assets": d.Assets, "services": d.Services, "software": d.Software} {
		if len(sec.Added) != 0 || len(sec.Removed) != 0 {
			t.Errorf("diff %s across two identical upgraded syncs = %+v, want no change", name, sec)
		}
	}
	if res, _ := ExposureDelta(ctx, db, 0); len(res.NewServices) != 0 || len(res.NewVulns) != 0 {
		t.Errorf("exposure-delta across two identical upgraded syncs = %+v %+v, want nothing new", res.NewServices, res.NewVulns)
	}
}
