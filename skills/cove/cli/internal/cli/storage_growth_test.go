// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
package cli

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestStorageGrowthComputesDeltas(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dbPath := seedSnapshots(t) // regressor: 1000→1500, others unchanged/new
	out, err := runCoveCmd(t, "storage", "growth", "--db", dbPath, "--json")
	if err != nil {
		t.Fatalf("storage growth: %v\n%s", err, out)
	}
	var view struct {
		Items []struct {
			DeviceName string  `json:"device_name"`
			FromBytes  int64   `json:"from_bytes"`
			ToBytes    int64   `json:"to_bytes"`
			DeltaBytes int64   `json:"delta_bytes"`
			DeltaPct   float64 `json:"delta_pct"`
		} `json:"items"`
		ByCustomer []struct {
			Customer   string `json:"customer"`
			DeltaBytes int64  `json:"delta_bytes"`
		} `json:"by_customer"`
		TotalDeltaBytes int64 `json:"total_delta_bytes"`
	}
	if err := json.Unmarshal([]byte(out), &view); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out)
	}
	// Only the device whose storage moved appears.
	if len(view.Items) != 1 || view.Items[0].DeviceName != "regressor" {
		t.Fatalf("expected only the grown device, got %+v", view.Items)
	}
	it := view.Items[0]
	if it.FromBytes != 1000 || it.ToBytes != 1500 || it.DeltaBytes != 500 || it.DeltaPct != 50.0 {
		t.Fatalf("delta math wrong: %+v", it)
	}
	if view.TotalDeltaBytes != 500 {
		t.Fatalf("total delta wrong: %d", view.TotalDeltaBytes)
	}
	if len(view.ByCustomer) == 0 || view.ByCustomer[0].Customer != "Acme" || view.ByCustomer[0].DeltaBytes != 500 {
		t.Fatalf("per-customer rollup wrong: %+v", view.ByCustomer)
	}
	// Brand-new device has no baseline — must not fabricate growth.
	if strings.Contains(out, "newcomer") {
		t.Fatalf("new device without baseline leaked into growth:\n%s", out)
	}
}

func TestStorageGrowthEmptyStoreGuidance(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	out, err := runCoveCmd(t, "storage", "growth", "--db", t.TempDir()+"/empty.db", "--json")
	if err != nil {
		t.Fatalf("storage growth empty: %v\n%s", err, out)
	}
	if !strings.Contains(out, "no snapshots yet") {
		t.Fatalf("expected no-snapshots guidance:\n%s", out)
	}
}

func TestStorageGrowthRejectsLiveDataSource(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dbPath := seedSnapshots(t)
	out, err := runCoveCmd(t, "storage", "growth", "--db", dbPath, "--data-source", "live", "--json")
	if err == nil {
		t.Fatalf("expected error for --data-source live on a local-only command, got:\n%s", out)
	}
}
