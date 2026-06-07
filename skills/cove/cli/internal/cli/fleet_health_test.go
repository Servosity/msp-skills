// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
package cli

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

func TestFleetHealthRollupCounts(t *testing.T) {
	now := time.Now()
	fresh := fmt.Sprint(now.Add(-2 * time.Hour).Unix())
	old := fmt.Sprint(now.Add(-10 * 24 * time.Hour).Unix())
	srv := mockFleet(t, []map[string]any{
		{"AccountId": 1, "Settings": settingsList(map[string]string{"I1": "h1", "I8": "Acme", "D9F00": "5", "D9F09": fresh})},
		{"AccountId": 2, "Settings": settingsList(map[string]string{"I1": "h2", "I8": "Acme", "D9F00": "2", "D9F09": fresh})},
		{"AccountId": 3, "Settings": settingsList(map[string]string{"I1": "h3", "I8": "Globex", "D9F00": "7"})},
		{"AccountId": 4, "Settings": settingsList(map[string]string{"I1": "h4", "I8": "Globex", "D9F00": "5", "D9F09": old})},
		// In-progress AND stale: overlay metric + stale bucket (totals reconcile).
		{"AccountId": 5, "Settings": settingsList(map[string]string{"I1": "h5", "I8": "Globex", "D9F00": "1", "D9F09": old})},
	})
	fleetTestEnv(t, srv)

	out, err := runCoveCmd(t, "fleet", "health", "--by", "partner", "--json")
	if err != nil {
		t.Fatalf("fleet health: %v\n%s", err, out)
	}
	var view struct {
		TotalDevices int            `json:"total_devices"`
		Healthy      int            `json:"healthy"`
		Failed       int            `json:"failed"`
		Stale        int            `json:"stale"`
		NeverRun     int            `json:"never_run"`
		InProgress   int            `json:"in_progress"`
		ByStatus     map[string]int `json:"by_status"`
		ByPartner    []struct {
			Customer string `json:"customer"`
			Total    int    `json:"total"`
			Healthy  int    `json:"healthy"`
			Failed   int    `json:"failed"`
			Stale    int    `json:"stale"`
			NeverRun int    `json:"never_run"`
		} `json:"by_partner"`
	}
	if err := json.Unmarshal([]byte(out), &view); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out)
	}
	if view.TotalDevices != 5 || view.Healthy != 1 || view.Failed != 1 || view.NeverRun != 1 || view.Stale != 2 || view.InProgress != 1 {
		t.Fatalf("rollup counts wrong: %+v", view)
	}
	// Primary buckets must reconcile: healthy+failed+stale+never_run == total.
	if view.Healthy+view.Failed+view.Stale+view.NeverRun != view.TotalDevices {
		t.Fatalf("primary buckets do not sum to total: %+v", view)
	}
	if view.ByStatus["Completed"] != 2 || view.ByStatus["Failed"] != 1 || view.ByStatus["NotStarted"] != 1 || view.ByStatus["InProcess"] != 1 {
		t.Fatalf("by_status decode wrong: %v", view.ByStatus)
	}
	// Acme has the failure → sorted first in the per-partner table.
	if len(view.ByPartner) != 2 || view.ByPartner[0].Customer != "Acme" || view.ByPartner[0].Failed != 1 {
		t.Fatalf("per-partner breakdown wrong: %+v", view.ByPartner)
	}
	// Per-partner buckets must also reconcile for every row.
	for _, ph := range view.ByPartner {
		if ph.Healthy+ph.Failed+ph.Stale+ph.NeverRun != ph.Total {
			t.Fatalf("per-partner buckets do not sum to total: %+v", ph)
		}
	}
}

func TestFleetHealthRejectsUnknownBreakdown(t *testing.T) {
	srv := mockFleet(t, nil)
	fleetTestEnv(t, srv)
	out, err := runCoveCmd(t, "fleet", "health", "--by", "region", "--json")
	if err == nil {
		t.Fatalf("expected usage error for --by region, got:\n%s", out)
	}
}
