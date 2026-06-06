// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written (Phase 3): behavioral tests for reconcile against a seeded store.

package cli

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestNovelReconcileCommand(t *testing.T) {
	db := seedTranscendenceStore(t)
	dbPath := db.Path()
	db.Close()

	cmd := newNovelReconcileCmd(&rootFlags{asJSON: true})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"--db", dbPath})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("reconcile execute: %v", err)
	}

	var res reconcileResult
	if err := json.Unmarshal(buf.Bytes(), &res); err != nil {
		t.Fatalf("parsing reconcile JSON: %v\n%s", err, buf.String())
	}
	if res.Summary.BilledWithoutSub < 1 {
		t.Errorf("expected >=1 billed-without-active-subscription, got %d", res.Summary.BilledWithoutSub)
	}
	if res.Summary.ActiveWithoutInvoiceN < 1 {
		t.Errorf("expected >=1 active-without-invoice, got %d", res.Summary.ActiveWithoutInvoiceN)
	}
	// c-billed was invoiced but has only a Cancelled subscription -> must be flagged.
	foundBilled := false
	for _, f := range res.BilledWithoutActiveSubscription {
		if f.CompanyID == "c-billed" {
			foundBilled = true
		}
	}
	if !foundBilled {
		t.Errorf("c-billed (invoice + only cancelled sub) should be billed_without_active_subscription")
	}
	// Negative: c-active is fully reconciled, must NOT be flagged either way.
	for _, f := range res.ActiveWithoutInvoice {
		if f.CompanyID == "c-active" {
			t.Errorf("c-active is reconciled; should not be active_without_invoice")
		}
	}
	for _, f := range res.BilledWithoutActiveSubscription {
		if f.CompanyID == "c-active" {
			t.Errorf("c-active has an active sub; should not be billed_without_active_subscription")
		}
	}
}
