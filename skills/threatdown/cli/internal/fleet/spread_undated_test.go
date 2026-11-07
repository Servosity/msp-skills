// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package fleet

import (
	"encoding/json"
	"testing"
	"time"
)

// An unreadable timestamp must never hide a detection from threats spread or
// the customer report's top threats (Codex r2 input of the 4.35.1 reprint).
func TestSpreadCountsUndatedDetections(t *testing.T) {
	var d Detection
	if err := json.Unmarshal([]byte(`{"id":"d1","account_id":"a1","machine_id":"e1","threat_name":"Trojan","reported_at":"yesterday-ish"}`), &d); err != nil {
		t.Fatal(err)
	}
	dated := Detection{AccountID: "a2", MachineID: "e2", ThreatName: "Trojan", ReportedAt: ago(3)}
	sites := map[string]Site{"a1": {Name: "Acme"}, "a2": {Name: "Globex"}}

	rows := Spread([]Detection{d}, sites, now.Add(-7*24*time.Hour), 1)
	if len(rows) != 1 || rows[0].Detections != 1 || rows[0].UndatedDetections != 1 || rows[0].Sites != 1 {
		t.Fatalf("undated detection hidden or miscounted: %+v", rows)
	}
	if rows[0].FirstSeen != "" || rows[0].LastSeen != "" {
		t.Fatalf("an undated-only group has no first/last seen: %+v", rows[0])
	}

	rows = Spread([]Detection{d, dated}, sites, now.Add(-7*24*time.Hour), 2)
	if len(rows) != 1 || rows[0].Sites != 2 || rows[0].Detections != 2 || rows[0].UndatedDetections != 1 {
		t.Fatalf("undated detection must count toward the cross-site spread: %+v", rows)
	}
	if want := now.Add(-3 * time.Hour).UTC().Format(time.RFC3339); rows[0].FirstSeen != want || rows[0].LastSeen != want {
		t.Fatalf("first/last seen come only from dated detections: %+v (want %s)", rows[0], want)
	}
}
