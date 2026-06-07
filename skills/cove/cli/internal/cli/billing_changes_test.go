// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
package cli

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBillingChangesFindsOnlySKUFlips(t *testing.T) {
	srv := mockFleet(t, []map[string]any{
		// Plan change: WKS → SRV (must appear).
		{"AccountId": 1, "Settings": settingsList(map[string]string{
			"I1": "upgraded", "I8": "Acme", "I57": "SRV", "I58": "WKS",
		})},
		// Unchanged (must be absent).
		{"AccountId": 2, "Settings": settingsList(map[string]string{
			"I1": "steady", "I8": "Acme", "I57": "DOC", "I58": "DOC",
		})},
		// New device with no prior month (must be absent — not a change).
		{"AccountId": 3, "Settings": settingsList(map[string]string{
			"I1": "brand-new", "I8": "Globex", "I57": "WKS", "I58": "",
		})},
	})
	fleetTestEnv(t, srv)

	out, err := runCoveCmd(t, "billing", "changes", "--json")
	if err != nil {
		t.Fatalf("billing changes: %v\n%s", err, out)
	}
	var view struct {
		Items []struct {
			DeviceName string `json:"device_name"`
			PrevSKU    string `json:"prev_sku"`
			SKU        string `json:"sku"`
		} `json:"items"`
		TotalChanges int `json:"total_changes"`
	}
	if err := json.Unmarshal([]byte(out), &view); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out)
	}
	if view.TotalChanges != 1 || len(view.Items) != 1 {
		t.Fatalf("expected exactly one SKU change, got %+v", view)
	}
	if view.Items[0].DeviceName != "upgraded" || view.Items[0].PrevSKU != "WKS" || view.Items[0].SKU != "SRV" {
		t.Fatalf("wrong change row: %+v", view.Items[0])
	}
	if strings.Contains(out, "steady") || strings.Contains(out, "brand-new") {
		t.Fatalf("non-changes leaked into output:\n%s", out)
	}
}

func TestBillingChangesZeroResultIsHonest(t *testing.T) {
	srv := mockFleet(t, []map[string]any{
		{"AccountId": 2, "Settings": settingsList(map[string]string{
			"I1": "steady", "I8": "Acme", "I57": "DOC", "I58": "DOC",
		})},
	})
	fleetTestEnv(t, srv)
	out, err := runCoveCmd(t, "billing", "changes", "--json")
	if err != nil {
		t.Fatalf("billing changes: %v\n%s", err, out)
	}
	if !strings.Contains(out, `"total_changes": 0`) || !strings.Contains(out, "no SKU changes") {
		t.Fatalf("expected honest zero-change envelope:\n%s", out)
	}
}
