// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Verifies fleet-wide search projects raw JSON:API records into clean,
// org-annotated summaries (the C1 differentiator).

package cli

import (
	"encoding/json"
	"testing"
)

func TestSearchProjection(t *testing.T) {
	seedITGStore(t)
	out := runITGCmd(t, "search", "Globex", "--json")

	var env struct {
		Results []map[string]any `json:"results"`
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("unmarshal: %v\noutput: %s", err, out)
	}
	if len(env.Results) == 0 {
		t.Fatalf("expected search hits for Globex; got %s", out)
	}
	for _, r := range env.Results {
		if _, ok := r["resource_type"]; !ok {
			t.Errorf("projected search hit missing resource_type: %#v", r)
		}
		if _, ok := r["id"]; !ok {
			t.Errorf("projected search hit missing id: %#v", r)
		}
	}

	// A hyphenated serial number must not crash the FTS MATCH parser; it
	// should match the configuration carrying that serial.
	out2 := runITGCmd(t, "search", "SN-ABC123", "--json")
	var env2 struct {
		Results []map[string]any `json:"results"`
	}
	if err := json.Unmarshal([]byte(out2), &env2); err != nil {
		t.Fatalf("unmarshal hyphen search: %v\noutput: %s", err, out2)
	}
	foundConfig := false
	for _, r := range env2.Results {
		if r["id"] == "30" && r["resource_type"] == "configurations" {
			foundConfig = true
		}
	}
	if !foundConfig {
		t.Errorf("search SN-ABC123 should find configuration 30; got %s", out2)
	}
}
