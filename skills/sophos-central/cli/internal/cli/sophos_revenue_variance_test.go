// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/sophos-central/handfixes.json
// under the "revenue-variance-per-product" entry.

package cli

import (
	"context"
	"encoding/json"
	"io"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"

	"sophos-central-pp-cli/internal/store"
)

// TestRevenueVarianceNotSummedAcrossProducts: ten machines with ten
// protection and ten encryption seats is not a ten-seat discrepancy.
func TestRevenueVarianceNotSummedAcrossProducts(t *testing.T) {
	lines := []billingLine{
		{AccountID: "t1", ProductCode: "ixe", BillableQuantity: 10, UnitPrice: "1.00", UnitPriceCurrency: "USD"},
		{AccountID: "t1", ProductCode: "encryption", BillableQuantity: 10, UnitPrice: "1.00", UnitPriceCurrency: "USD"},
	}
	view := buildBilling("2026-09", lines, map[string]string{"t1": "Acme"}, map[string]int{"t1": 10})
	if len(view.Items) != 1 || view.Items[0].Variance == nil || *view.Items[0].Variance != 0 {
		t.Fatalf("customer variance = %v, want 0", view.Items[0].Variance)
	}
	for _, p := range view.Items[0].Products {
		if p.BillableMinusDeployed == nil || *p.BillableMinusDeployed != 0 {
			t.Fatalf("product %s variance = %v, want 0", p.Product, p.BillableMinusDeployed)
		}
	}
}

func TestZeroEndpointTenantIsDeployedZero(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "data.db")
	db, err := store.OpenWithContext(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	row, _ := json.Marshal(fleetLedgerRow{ID: "T9", Status: "ok", Resources: map[string]fleetOutcome{"endpoints": {Status: "ok"}}})
	if _, _, err := db.UpsertBatch(fleetLedgerType, []json.RawMessage{row}); err != nil {
		t.Fatal(err)
	}
	db.Close()
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	cmd.SetErr(io.Discard)
	if _, deployed := billingJoinData(cmd, dbPath); deployed["T9"] != 0 || len(deployed) != 1 {
		t.Fatalf("a refreshed tenant with no endpoints is deployed 0, not missing: %v", deployed)
	}
}
