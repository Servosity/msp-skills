// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"testing"
)

func TestFlattenSnapshotRows(t *testing.T) {
	view := snapshotView{
		Sections: map[string]json.RawMessage{
			"attack_stopped": json.RawMessage(`{"total": 42, "byType": {"phishing": 30, "bec": 12}}`),
			"trending":       json.RawMessage(`{"attacks": [{"name": "wire fraud", "count": 5}]}`),
		},
	}
	rows := flattenSnapshotRows(view)
	if len(rows) == 0 {
		t.Fatalf("expected flattened rows, got none")
	}
	found := map[string]any{}
	for _, row := range rows {
		section, _ := row["section"].(string)
		metric, _ := row["metric"].(string)
		found[section+"/"+metric] = row["value"]
	}
	if v, ok := found["attack_stopped/total"]; !ok || v.(float64) != 42 {
		t.Errorf("missing or wrong attack_stopped/total: %v", v)
	}
	if v, ok := found["attack_stopped/byType.phishing"]; !ok || v.(float64) != 30 {
		t.Errorf("missing or wrong nested metric: %v", v)
	}
	if v, ok := found["trending/attacks[0].name"]; !ok || v != "wire fraud" {
		t.Errorf("missing or wrong array metric: %v", v)
	}
	// Every row carries the three CSV columns.
	for _, row := range rows {
		for _, k := range []string{"section", "metric", "value"} {
			if _, ok := row[k]; !ok {
				t.Errorf("row missing %q: %v", k, row)
			}
		}
	}
}

func TestFlattenSnapshotRowsCarriesFetchFailures(t *testing.T) {
	view := snapshotView{
		Sections: map[string]json.RawMessage{
			"attack_stopped": json.RawMessage(`{"total": 1}`),
		},
		FetchFailures: []snapshotFailure{
			{Section: "trending_attacks", Error: "HTTP 429"},
		},
	}
	rows := flattenSnapshotRows(view)
	foundFailure := false
	for _, row := range rows {
		if row["section"] == "trending_attacks" && row["metric"] == "fetch_error" && row["value"] == "HTTP 429" {
			foundFailure = true
		}
	}
	if !foundFailure {
		t.Errorf("CSV rows must carry failed sections as fetch_error rows, got %v", rows)
	}
}

func TestFlattenSnapshotRowsSkipsBadJSON(t *testing.T) {
	view := snapshotView{
		Sections: map[string]json.RawMessage{
			"broken": json.RawMessage(`{not json`),
			"good":   json.RawMessage(`{"n": 1}`),
		},
	}
	rows := flattenSnapshotRows(view)
	if len(rows) != 1 {
		t.Fatalf("expected exactly 1 row from the good section, got %d", len(rows))
	}
	if rows[0]["section"] != "good" {
		t.Errorf("unexpected section: %v", rows[0])
	}
}

func TestSnapshotSectionsStable(t *testing.T) {
	secs := snapshotSections()
	if len(secs) != 6 {
		t.Fatalf("expected 6 snapshot sections, got %d", len(secs))
	}
	seen := map[string]bool{}
	for _, s := range secs {
		if s.key == "" || s.path == "" {
			t.Errorf("section with empty key/path: %+v", s)
		}
		if seen[s.key] {
			t.Errorf("duplicate section key %q", s.key)
		}
		seen[s.key] = true
	}
}
