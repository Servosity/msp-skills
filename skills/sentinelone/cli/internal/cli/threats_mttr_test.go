// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"testing"
	"time"
)

func TestThreatsMTTRComputesDurationsAndBreaches(t *testing.T) {
	now := time.Now().UTC()
	threats := []map[string]any{
		// Mitigated in ~5h.
		threatFix("t1", "Quick", "h1", "BOX-1", "SiteA", map[string]any{
			"mitigationStatus": "mitigated",
			"incidentStatus":   "resolved",
			"createdAt":        now.Add(-5 * time.Hour).Format(time.RFC3339),
			"updatedAt":        now.Format(time.RFC3339),
		}),
		// Still active for ~30h — an SLA breach at sla=24.
		threatFix("t2", "Lingering", "h2", "BOX-2", "SiteA", map[string]any{
			"mitigationStatus": "active",
			"incidentStatus":   "unresolved",
			"createdAt":        now.Add(-30 * time.Hour).Format(time.RFC3339),
		}),
	}
	db := newSeededStore(t, nil, threats)
	out := runJSON(t, newNovelThreatsMttrCmd(jsonFlags()), "--db", db, "--sla", "24")
	if m, _ := out["total_mitigated"].(float64); m != 1 {
		t.Fatalf("total_mitigated = %v, want 1", out["total_mitigated"])
	}
	if h, _ := out["overall_mttr_hours"].(float64); h < 4.5 || h > 5.5 {
		t.Fatalf("overall_mttr_hours = %v, want ~5", out["overall_mttr_hours"])
	}
	site := out["sites"].([]any)[0].(map[string]any)
	if a, _ := site["active"].(float64); a != 1 {
		t.Fatalf("active = %v, want 1", site["active"])
	}
	if b, _ := site["sla_breaches"].(float64); b < 1 {
		t.Fatalf("sla_breaches = %v, want >= 1", site["sla_breaches"])
	}
}

func TestThreatsMTTRMeanIgnoresTimestamplessMitigations(t *testing.T) {
	now := time.Now().UTC()
	threats := []map[string]any{
		// Mitigated in ~6h with full timestamps.
		threatFix("m1", "Timed", "h1", "BOX-1", "SiteA", map[string]any{
			"mitigationStatus": "mitigated",
			"incidentStatus":   "resolved",
			"createdAt":        now.Add(-6 * time.Hour).Format(time.RFC3339),
			"updatedAt":        now.Format(time.RFC3339),
		}),
		// Mitigated but with no usable timestamps: counted, never averaged.
		threatFix("m2", "Stampless", "h2", "BOX-2", "SiteA", map[string]any{
			"mitigationStatus": "mitigated",
			"incidentStatus":   "resolved",
			"createdAt":        "",
			"updatedAt":        "",
		}),
	}
	db := newSeededStore(t, nil, threats)
	out := runJSON(t, newNovelThreatsMttrCmd(jsonFlags()), "--db", db, "--sla", "24")
	if m, _ := out["total_mitigated"].(float64); m != 2 {
		t.Fatalf("total_mitigated = %v, want 2", out["total_mitigated"])
	}
	// Mean must be ~6h (measured-only), not ~3h (diluted by the stampless one).
	if h, _ := out["overall_mttr_hours"].(float64); h < 5.5 || h > 6.5 {
		t.Fatalf("overall_mttr_hours = %v, want ~6 (mean must ignore stampless mitigations)", out["overall_mttr_hours"])
	}
}
