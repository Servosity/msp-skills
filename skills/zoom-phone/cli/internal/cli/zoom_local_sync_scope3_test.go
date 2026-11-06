// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Codex round-5: provenance from real completion, authoritative device
// partitions, per-site location coverage.

package cli

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"zoom-phone-pp-cli/internal/store"
)

// ---- item 1: a run that completes nothing records nothing ----

func TestLocalFailedSyncRecordsNoProvenance(t *testing.T) {
	localTestEnv(t)
	var mu sync.Mutex
	failNumbers := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch strings.TrimPrefix(r.URL.Path, "/v2") {
		case "/phone/numbers":
			mu.Lock()
			fail := failNumbers
			mu.Unlock()
			if fail {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"code":300,"message":"fixture failure"}`))
				return
			}
			_, _ = w.Write([]byte(`{"phone_numbers":[` + localTestNumbers[0] + `],"next_page_token":""}`))
		default:
			_, _ = w.Write([]byte(`{"next_page_token":""}`))
		}
	}))
	defer srv.Close()
	t.Setenv("ZOOM_PHONE_BASE_URL", srv.URL+"/v2")
	db := filepath.Join(t.TempDir(), "data.db")

	localRunSync(t, "--full", "--resources", "numbers", "--db", db, "--json")
	if scope := localScopeRows(t, db); scope["numbers"] != "0|1|" {
		t.Fatalf("setup scope = %v", scope)
	}
	// Fails --resource-param validation before any checkpoint is touched.
	localRunSync(t, "--full", "--resources", "numbers", "--db", db, "--resource-param", "typo:key=value", "--json")
	if scope := localScopeRows(t, db); scope["numbers"] != "0|1|" {
		t.Fatalf("a run that failed validation rewrote numbers provenance: %v", scope)
	}
	// Starts the numbers attempt, then the fetch fails: still no completion.
	// Without --full the attempt keeps the old (non-zero) watermark, so only
	// the completion check stops it from being recorded.
	mu.Lock()
	failNumbers = true
	mu.Unlock()
	for _, extra := range [][]string{{}, {"--full"}} {
		args := append(append([]string{}, extra...), "--resources", "numbers", "--db", db, "--json")
		localRunSync(t, args...)
		if scope := localScopeRows(t, db); scope["numbers"] != "0|1|" {
			t.Fatalf("a run (%v) whose numbers fetch failed rewrote numbers provenance: %v", extra, scope)
		}
	}
}

// ---- item 2: device partitions are authoritative ----

func TestLocalGenericDeviceRowsOnlyEnrichPartitions(t *testing.T) {
	localTestEnv(t)
	rows := localTestCompleteRows()
	rows["assigned"] = append(append([]string{}, rows["assigned"]...),
		`{"id":"dev-nofw","display_name":"Front Desk","device_type":"Poly VVX 450","mac_address":"64-16-7f-00-00-09","status":"online","provision_template_id":"tpl-2","site":`+localTestHQ+`,"assignees":[{"id":"u-bob","name":"Bob Smith","extension_type":"user","extension_id":"ext-bob"}]}`)
	rows["devices"] = []string{
		// Not in either reconciled partition (e.g. removed since): ignored.
		`{"id":"dev-ghost","display_name":"Old Phone","device_type":"Yealink T54W","mac_address":"80-5e-c0-00-00-99","status":"offline","firmware_version":"90.0","site":` + localTestHQ + `,"assignees":[{"id":"u-jane","name":"Jane Doe","extension_type":"user","extension_id":"ext-jane"}]}`,
		// The unassigned partition says dev-spare has no assignee; the
		// generic row cannot change that.
		`{"id":"dev-spare","display_name":"Spare T54W","device_type":"Yealink T54W","status":"online","firmware_version":"99.0","site":` + localTestHQ + `,"assignees":[{"id":"u-jane","name":"Jane Doe","extension_type":"user","extension_id":"ext-jane"}]}`,
		// Fills the firmware the partition row lacks.
		`{"id":"dev-nofw","firmware_version":"6.4.0.1","status":"offline"}`,
	}
	db := localTestStore(t, rows)

	stdout, _, err := localRun(t, newNovelDevicesHealthCmd, "--db", db)
	if err != nil {
		t.Fatalf("devices health: %v", err)
	}
	var dh localDevicesHealthResult
	localDecode(t, stdout, &dh)
	byID := map[string]localDeviceFinding{}
	for _, sh := range dh.BySite {
		for _, f := range sh.Findings {
			byID[f.DeviceID] = f
		}
	}
	if _, ok := byID["dev-ghost"]; ok || dh.DevicesChecked != 6 || !dh.Assessed {
		t.Fatalf("generic-only device reported: ghost=%v checked=%d assessed=%v", byID["dev-ghost"], dh.DevicesChecked, dh.Assessed)
	}
	if f := byID["dev-spare"]; !strings.Contains(strings.Join(f.Flags, ","), "UNASSIGNED") || f.FirmwareVersion != "96.86.0.30" || f.Status != "offline" {
		t.Fatalf("dev-spare = %+v; the partition row must win on assignment, firmware and status", f)
	}
	var yealinkVersions map[string]int
	for _, b := range dh.FirmwareBaselines {
		if b.DeviceType == "Poly VVX 450" && b.Versions["6.4.0.1"] != 2 {
			t.Fatalf("dev-nofw firmware not enriched from the generic row: %+v", b)
		}
		if b.DeviceType == "Yealink T54W" {
			yealinkVersions = b.Versions
		}
	}
	if yealinkVersions["90.0"] != 0 || yealinkVersions["99.0"] != 0 {
		t.Fatalf("generic rows leaked into the firmware baseline: %v", yealinkVersions)
	}

	stdout, _, err = localRun(t, newNovelFootprintCmd, "jane@example.com", "--db", db, "--no-live")
	if err != nil {
		t.Fatalf("footprint: %v", err)
	}
	var fp localFootprint
	localDecode(t, stdout, &fp)
	if len(fp.DeskPhones) != 1 || fp.DeskPhones[0].ID != "dev-jane" {
		t.Fatalf("desk phones = %+v, want only dev-jane (no ghost, no spare)", fp.DeskPhones)
	}
}

// ---- item 3: locations covered site by site ----

func localE911FromStore(t *testing.T, db string) localE911Result {
	t.Helper()
	stdout, _, err := localRun(t, newNovelE911AuditCmd, "--db", db)
	if err != nil {
		t.Fatalf("e911: %v", err)
	}
	var e localE911Result
	localDecode(t, stdout, &e)
	return e
}

func localFindingSites(e localE911Result, kind string) string {
	out := []string{}
	for _, f := range e.Findings {
		if f.Kind == kind {
			out = append(out, f.EntityID)
		}
	}
	return strings.Join(out, ",")
}

func localContains(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}

func TestLocalLocationsSingleSiteUnfiltered(t *testing.T) {
	localTestEnv(t)
	e := localE911FromStore(t, localTestStore(t, localTestE911Rows()))
	if localContains(e.MissingLocal, "locations") || localFindingSites(e, "SITE_NO_ERL") != "site-au,site-br" {
		t.Fatalf("unfiltered full locations: missing=%v SITE_NO_ERL=%s", e.MissingLocal, localFindingSites(e, "SITE_NO_ERL"))
	}
}

func TestLocalLocationsMultiSitePerSiteSyncs(t *testing.T) {
	localTestEnv(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch strings.TrimPrefix(r.URL.Path, "/v2") {
		case "/phone/sites":
			_, _ = w.Write([]byte(`{"sites":[{"id":"site-hq","name":"San Jose HQ"},{"id":"site-br","name":"Boston Branch"}],"next_page_token":""}`))
		case "/phone/locations":
			switch r.URL.Query().Get("site_id") {
			case "site-hq":
				_, _ = w.Write([]byte(`{"locations":[{"id":"loc-hq","name":"HQ Floor 1","site":{"id":"site-hq","name":"San Jose HQ"}}],"next_page_token":""}`))
			case "site-br":
				_, _ = w.Write([]byte(`{"locations":[],"next_page_token":""}`))
			default: // Multiple Sites: site_id is required
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"code":300,"message":"site_id is required"}`))
			}
		default:
			_, _ = w.Write([]byte(`{"next_page_token":""}`))
		}
	}))
	defer srv.Close()
	t.Setenv("ZOOM_PHONE_BASE_URL", srv.URL+"/v2")
	db := filepath.Join(t.TempDir(), "data.db")

	localRunSync(t, "--full", "--resources", "sites", "--db", db, "--json")
	localRunSync(t, "--resources", "locations", "--db", db, "--resource-param", "locations:site_id=site-hq", "--json")

	// One site walked: Boston is uncovered, so its SITE_NO_ERL is skipped.
	e := localE911FromStore(t, db)
	if !localContains(e.MissingLocal, "locations") || !strings.Contains(e.MissingReasons["locations"], "site-br") ||
		!strings.Contains(e.MissingReasons["locations"], "--resource-param locations:site_id=site-br") {
		t.Fatalf("one site walked: missing=%v reason=%q", e.MissingLocal, e.MissingReasons["locations"])
	}
	if localFindingSites(e, "SITE_NO_ERL") != "" || !strings.Contains(strings.Join(e.SkippedChecks, "\n"), "SITE_NO_ERL: locations not synced for site(s) site-br") {
		t.Fatalf("one site walked: SITE_NO_ERL=%q skipped=%v", localFindingSites(e, "SITE_NO_ERL"), e.SkippedChecks)
	}

	// Both sites walked: locations covered; Boston has no ERL.
	localRunSync(t, "--resources", "locations", "--db", db, "--resource-param", "locations:site_id=site-br", "--json")
	e = localE911FromStore(t, db)
	if localContains(e.MissingLocal, "locations") || localFindingSites(e, "SITE_NO_ERL") != "site-br" ||
		strings.Contains(strings.Join(e.SkippedChecks, "\n"), "SITE_NO_ERL") {
		t.Fatalf("both sites walked: missing=%v SITE_NO_ERL=%q skipped=%v", e.MissingLocal, localFindingSites(e, "SITE_NO_ERL"), e.SkippedChecks)
	}

	// A filtered --full is additive on press 4.34.2 (a filtered sync never
	// prunes), so the other site's rows and coverage stay.
	localRunSync(t, "--full", "--resources", "locations", "--db", db, "--resource-param", "locations:site_id=site-hq", "--json")
	e = localE911FromStore(t, db)
	if localContains(e.MissingLocal, "locations") || localFindingSites(e, "SITE_NO_ERL") != "site-br" {
		t.Fatalf("after a site-hq --full: missing=%v reason=%q SITE_NO_ERL=%q", e.MissingLocal, e.MissingReasons["locations"], localFindingSites(e, "SITE_NO_ERL"))
	}
}

func TestLocalLocationsPerSiteFreshness(t *testing.T) {
	localTestEnv(t)
	rows := localTestE911Rows()
	rows["sites"] = rows["sites"][:2] // HQ and Boston
	rows["locations"] = []string{
		`{"id":"loc-hq","name":"HQ Floor 1","site":` + localTestHQ + `}`,
		`{"id":"loc-br-old","name":"Boston (removed)","site":` + localTestBoston + `}`,
	}
	path := localTestStore(t, rows)
	db, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	var sitesAt string
	if err := db.DB().QueryRow(`SELECT synced_at FROM zoom_sync_scope WHERE resource = 'sites'`).Scan(&sitesAt); err != nil {
		t.Fatal(err)
	}
	// Locations synced site by site (the latest record is filtered).
	if err := localWriteSyncScope(db, "locations", 1, "site_id=site-br", sitesAt, "now", false); err != nil {
		t.Fatal(err)
	}
	// Boston's walk returned nothing, so its old row (still stored) was not
	// returned by it and is stale. HQ's walk is older than the sites sync.
	if err := localWriteWalk(db, "locations", "site_id=site-br", true, false, sitesAt, "now", nil); err != nil {
		t.Fatal(err)
	}
	if err := localWriteWalk(db, "locations", "site_id=site-hq", true, false, "2001-01-01T00:00:00Z", "then", []string{"loc-hq"}); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()

	e := localE911FromStore(t, path)
	if localFindingSites(e, "SITE_NO_ERL") != "site-br" {
		t.Fatalf("stale Boston row must not count as an ERL: SITE_NO_ERL=%q", localFindingSites(e, "SITE_NO_ERL"))
	}
	if !localContains(e.MissingLocal, "locations") || !strings.Contains(e.MissingReasons["locations"], "site-hq") || strings.Contains(e.MissingReasons["locations"], "site-br") {
		t.Fatalf("HQ's record is older than the sites sync: missing=%v reason=%q", e.MissingLocal, e.MissingReasons["locations"])
	}
}
