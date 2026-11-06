// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
package syncer

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"afi-pp-cli/internal/store"
)

// Class sync-snapshot-read-as-current (afi handfix fleet-sync-snapshot-truth):
// a re-sync over the real store removes what the API no longer returns once the
// list that would carry it was read in full, and a list it could not read in
// full keeps its rows and is recorded as a gap until a later complete read.

func openPruneStore(t *testing.T) *store.Store {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func countRows(t *testing.T, db *store.Store, rtype string) int {
	t.Helper()
	var n int
	if err := db.DB().QueryRow(`SELECT COUNT(*) FROM resources WHERE resource_type = ?`, rtype).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func gapIDs(t *testing.T, db *store.Store) map[string]bool {
	t.Helper()
	rows, err := db.DB().Query(`SELECT id FROM resources WHERE resource_type = ?`, GapResourceType)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		out[id] = true
	}
	return out
}

func runOn(t *testing.T, api *fakeAPI, db *store.Store, opts Options) *Summary {
	t.Helper()
	opts.Now = fixedNow
	sum, err := Run(context.Background(), api, StoreSink{Store: db}, opts)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	return sum
}

func TestResyncRemovesWhatTheAPINoLongerReturns(t *testing.T) {
	db := openPruneStore(t)
	runOn(t, fleetFixture(), db, Options{})
	if countRows(t, db, "protections") != 1 {
		t.Fatal("setup: the first sync stored no protection")
	}
	// The protection was released in Afi (offboard, or the portal).
	api := fleetFixture()
	api.responses["/api/v1/tenants/ten1/protections"] = `{"items":[],"next_page_token":""}`
	runOn(t, api, db, Options{})
	if n := countRows(t, db, "protections"); n != 0 {
		t.Fatalf("a released protection still reads as current: %d row(s)", n)
	}
	// The tenant left the org: it and every collection under it go.
	api.responses["/api/v1/orgs/org1/tenants"] = `{"items":[],"next_page_token":""}`
	runOn(t, api, db, Options{})
	for _, rt := range []string{"tenants", "resources", "policies", "archives", "quotas", "task_stats"} {
		if n := countRows(t, db, rt); n != 0 {
			t.Errorf("%s of a removed tenant still stored: %d row(s)", rt, n)
		}
	}
	if countRows(t, db, "orgs") != 1 || countRows(t, db, "installations") != 1 {
		t.Error("the org and its installation are still listed and must stay")
	}
}

func TestIncompleteListIsKeptAndRecordedAsAGap(t *testing.T) {
	db := openPruneStore(t)
	runOn(t, fleetFixture(), db, Options{})

	api := fleetFixture()
	api.errors = map[string]error{"/api/v1/tenants/ten1/protections": fmt.Errorf("429 too many requests")}
	sum := runOn(t, api, db, Options{})
	if countRows(t, db, "protections") != 1 {
		t.Error("a failed read dropped the stored protections")
	}
	if !gapIDs(t, db)["ten1:protections"] || len(sum.Incomplete) != 1 {
		t.Errorf("the failed read is not recorded: gaps %v, summary %v", gapIDs(t, db), sum.Incomplete)
	}

	// A list cut at --max-pages keeps what it read and is recorded too.
	api = fleetFixture()
	api.responses["/api/v1/tenants/ten1/resources"] = `{"items":[{"id":"res1","tenant_id":"ten1","name":"Jane Doe","kind":"user"}],"next_page_token":"p2"}`
	runOn(t, api, db, Options{MaxPages: 1})
	if countRows(t, db, "resources") != 2 {
		t.Error("a capped read pruned the resources it did not reach")
	}
	gaps := gapIDs(t, db)
	if !gaps["ten1:resources"] || gaps["ten1:protections"] {
		t.Errorf("gaps after the capped run: %v (want resources, protections cleared by its complete read)", gaps)
	}

	// Skipped archives are stale by choice, and say so.
	runOn(t, fleetFixture(), db, Options{SkipArchives: true})
	if gaps := gapIDs(t, db); !gaps["ten1:archives"] || len(gaps) != 1 {
		t.Errorf("gaps after --skip-archives: %v", gaps)
	}

	// A tenants list that fails keeps the tenant (it may still exist) and is a fleet gap.
	api = fleetFixture()
	api.errors = map[string]error{"/api/v1/orgs/org1/tenants": fmt.Errorf("503 unavailable")}
	runOn(t, api, db, Options{})
	if countRows(t, db, "tenants") != 1 || countRows(t, db, "resources") != 2 {
		t.Error("a failed tenants read pruned the tenant")
	}
	if !gapIDs(t, db)["fleet:org org1 tenants"] {
		t.Errorf("the failed tenants read is not a fleet gap: %v", gapIDs(t, db))
	}

	runOn(t, fleetFixture(), db, Options{})
	if gaps := gapIDs(t, db); len(gaps) != 0 {
		t.Errorf("a complete sync left gaps: %v", gaps)
	}
}

// Codex r1 (afi review) class failed-sync-read-as-complete: a run that stops
// before its end (here the installations read, the root of the walk, fails)
// leaves a fleet gap, so every report says the latest refresh did not finish;
// the next complete run clears it.
func TestAFailedSyncLeavesAGap(t *testing.T) {
	db := openPruneStore(t)
	runOn(t, fleetFixture(), db, Options{})
	api := fleetFixture()
	api.errors = map[string]error{"/api/v1/applications/me/installations": fmt.Errorf("429 too many requests")}
	if _, err := Run(context.Background(), api, StoreSink{Store: db}, Options{Now: fixedNow}); err == nil {
		t.Fatal("a failed installations read must fail the run")
	}
	if !gapIDs(t, db)["fleet:fleet-sync"] {
		t.Errorf("the unfinished run left no gap: %v", gapIDs(t, db))
	}
	runOn(t, fleetFixture(), db, Options{})
	if gaps := gapIDs(t, db); len(gaps) != 0 {
		t.Errorf("a complete run left gaps: %v", gaps)
	}
}
