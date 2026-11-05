// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// Next-batch r1 (handfixes.json hubspot-velocity-history-must-describe-current-stage):
// stored stage history dates a deal's current stage only when it was captured
// no earlier than the deal's last change (hs_lastmodifieddate). A deal that
// left its stage and came back after the capture (PATCH /crm/v3/objects/0-3,
// press/tools-manifest.json) carries the same stage as an old entry, so a
// matching stage alone proves nothing.

import (
	"encoding/json"
	"testing"

	"hubspot-pp-cli/internal/cliutil/testenv"
	"hubspot-pp-cli/internal/store"
)

func nbHistoryCapturedAt(t *testing.T, s *store.Store, id, stage, ts, capturedAt string) {
	t.Helper()
	if _, err := s.DB().Exec(`INSERT INTO hubspot_property_history (object_type, object_id, property, value, timestamp, source, snapshot_at) VALUES ('deals', ?, 'dealstage', ?, ?, 'CRM_UI', ?)`, id, stage, ts, capturedAt); err != nil {
		t.Fatal(err)
	}
}

func TestNBVelocityHistoryMustPostdateTheDealsLastChange(t *testing.T) {
	testenv.Isolate(t)
	s, path := r1Store(t)
	r3StatePipeline(t, s)
	// A -> B -> A after the capture: history captured August 1 still ends at
	// qualifiedtobuy, but the deal changed on October 20.
	r1Deal(t, s, "1", map[string]any{"dealname": "Came back", "dealstage": "qualifiedtobuy", "pipeline": "default", "hs_lastmodifieddate": "2026-10-20T12:00:00.000Z"})
	nbHistoryCapturedAt(t, s, "1", "qualifiedtobuy", "2026-07-01T00:00:00Z", "2026-08-01 00:00:00")
	// Kept: captured after the last change, the history dates the stage.
	r1Deal(t, s, "2", map[string]any{"dealname": "Fresh", "dealstage": "qualifiedtobuy", "pipeline": "default", "hs_lastmodifieddate": "2026-09-01T00:00:00Z"})
	nbHistoryCapturedAt(t, s, "2", "qualifiedtobuy", "2026-09-01T00:00:00Z", "2026-09-02 00:00:00")
	// No last-change time: nothing proves the capture is current.
	r1Deal(t, s, "3", map[string]any{"dealname": "Unproven", "dealstage": "qualifiedtobuy", "pipeline": "default"})
	nbHistoryCapturedAt(t, s, "3", "qualifiedtobuy", "2026-09-01T00:00:00Z", "2026-09-02 00:00:00")
	s.Close()

	out, stderr, err := r3Run(t, "deals", "velocity", "--json", "--db", path)
	if err != nil {
		t.Fatalf("deals velocity: %v\n%s", err, stderr)
	}
	var view r3VelocityView
	if err := json.Unmarshal([]byte(out), &view); err != nil {
		t.Fatalf("decode %q: %v", out, err)
	}
	if len(view.Deals) != 1 || view.Deals[0].ID != "2" {
		t.Fatalf("deals = %+v, want only deal 2 (history captured before a deal's last change must not date its stage)\n%s", view.Deals, out)
	}
	if view.SkippedByReason["stale_history"] != 2 {
		t.Fatalf("skipped_by_reason = %v, want 2 stale_history\n%s", view.SkippedByReason, out)
	}
}
