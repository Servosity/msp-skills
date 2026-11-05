// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// Nextfix2 (handfixes.json hubspot-velocity-history-must-describe-current-stage):
// a history row carries the snapshot time HubSpot reported with it (the
// record's hs_lastmodifieddate in the same propertiesWithHistory response,
// press/tools-manifest.json GET /crm/v3/objects/0-3), never the time it was
// written locally. A deal that changed while that response was in flight
// (PATCH /crm/v3/objects/0-3) is newer than the snapshot, so its history no
// longer dates its stage.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"hubspot-pp-cli/internal/cliutil/testenv"
)

func TestNB2VelocityHistoryCarriesTheVendorSnapshotTime(t *testing.T) {
	testenv.Isolate(t)
	snap := time.Now().UTC().Add(-2 * time.Hour).Truncate(time.Second)
	moved := snap.Add(2 * time.Second) // changed, and changed back, after the snapshot
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/crm/v3/objects/0-3" {
			_, _ = fmt.Fprint(w, `{"results":[]}`)
			return
		}
		withHistory := r.URL.Query().Get("propertiesWithHistory") != ""
		deal := func(id, modified string) string {
			s := fmt.Sprintf(`{"id":%q,"properties":{"dealname":"D%s","amount":"100","dealstage":"qualifiedtobuy","pipeline":"default","hs_is_closed":"false","hs_lastmodifieddate":%q},"createdAt":"2026-01-01T00:00:00Z","updatedAt":%q,"archived":false`, id, id, modified, modified)
			if withHistory {
				s += `,"propertiesWithHistory":{"dealstage":[{"value":"qualifiedtobuy","timestamp":"2026-08-01T00:00:00.000Z","sourceType":"CRM_UI"}]}`
			}
			return s + "}"
		}
		first := snap.Format(time.RFC3339Nano)
		if !withHistory {
			first = moved.Format(time.RFC3339Nano)
		}
		_, _ = fmt.Fprint(w, nbHonourProperties(r, `{"results":[`+deal("1", first)+`,`+deal("2", snap.Format(time.RFC3339Nano))+`]}`))
	}))
	t.Cleanup(srv.Close)
	t.Setenv("HUBSPOT_BASE_URL", srv.URL)
	t.Setenv("HUBSPOT_ACCESS_TOKEN", "pat-na1-"+"7f3c9e2a-b14d-4c8e-9a6f-2d5e8b1c7a33")
	path := filepath.Join(t.TempDir(), "velocity.db")
	for _, args := range [][]string{
		{"sync", "--resources", "hubspot-deals-crm", "--with-history", "dealstage", "--db", path, "--json"},
		{"sync", "--resources", "hubspot-deals-crm", "--full", "--db", path, "--json"},
	} {
		if _, stderr, err := r3dRun(t, args...); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, stderr)
		}
	}
	out, stderr, err := r3Run(t, "deals", "velocity", "--json", "--db", path)
	if err != nil {
		t.Fatalf("deals velocity: %v\n%s", err, stderr)
	}
	var view r3VelocityView
	if err := json.Unmarshal([]byte(out), &view); err != nil {
		t.Fatalf("decode %q: %v", out, err)
	}
	if len(view.Deals) != 1 || view.Deals[0].ID != "2" {
		t.Fatalf("deals = %+v, want only deal 2: deal 1 changed after the snapshot its history came with\n%s", view.Deals, out)
	}
	if view.SkippedByReason["stale_history"] != 1 {
		t.Fatalf("skipped_by_reason = %v, want 1 stale_history\n%s", view.SkippedByReason, out)
	}
}
