// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"testing"
)

// pax8 handfix novel-documented-shapes, Codex r2 classes fixed after the stop rule (UNREVIEWED by Codex).

// r2 class stale-alias-overrides-fresh-data: a later write-through row under
// another name of the same collection is discarded.
func TestR2NewerUsageSummaryWins(t *testing.T) {
	path, db := novelStore(t)
	seedAsSync(t, db, "subscriptions_usage_summaries", "subscriptions", sidA,
		`{"id":"00000000-0000-4000-8000-000000000001","companyId":"`+cid1+`","productId":"`+pid1+`","currentCharges":10}`,
		`{"id":"00000000-0000-4000-8000-000000000002","companyId":"`+cid1+`","productId":"`+pid1+`","currentCharges":10}`)
	if err := db.UpsertUsageSummaries(json.RawMessage(`{"id":"00000000-0000-4000-8000-000000000002","companyId":"` + cid1 + `","productId":"` + pid1 + `","currentCharges":90}`)); err != nil {
		t.Fatal(err)
	}
	var flagged []overageRow
	runNovelJSON(t, path, &flagged, "overage")
	if len(flagged) != 1 || flagged[0].CurrentCharges != 90 {
		t.Fatalf("overage = %+v, want the refreshed 90-charge summary flagged", flagged)
	}
}

// r2 class documented-status-misread: a discounted (paid) trial with no
// invoice line is unbilled.
func TestR2PaidTrialWithoutLineIsUnbilled(t *testing.T) {
	path, db := novelStore(t)
	seedAsSync(t, db, "subscriptions", "", "",
		`{"id":"`+sidA+`","companyId":"`+cid1+`","productId":"`+pid1+`","status":"Trial","billingTerm":"Monthly","billingStart":"2026-10-01","quantity":1,"price":5,"partnerCost":3}`)
	var got reconcileResult
	runNovelJSON(t, path, &got, "reconcile")
	if len(got.ActiveWithoutInvoice) != 1 {
		t.Fatalf("the paid trial produced no missing-invoice finding: %+v", got)
	}
}

// r2 class wrong-field-read (InvoiceItem.type / sign): a credit line is not
// billing after cancellation.
func TestR2CreditLineIsNotBilling(t *testing.T) {
	path, db := novelStore(t)
	seedAsSync(t, db, "subscriptions", "", "",
		`{"id":"`+sidA+`","companyId":"`+cid1+`","productId":"`+pid1+`","status":"Cancelled","billingTerm":"Monthly","endDate":"2026-09-15T00:00:00Z","quantity":1,"price":5}`)
	seedAsSync(t, db, "invoices", "", "", `{"id":"55555555-5555-4555-8555-555555555555","companyId":"`+cid1+`","total":-50}`)
	seedAsSync(t, db, "items", "invoices", "55555555-5555-4555-8555-555555555555",
		`{"id":"44444444-4444-4444-8444-444444444441","type":"service-credit","companyId":"`+cid1+`","subscriptionId":"`+sidA+`","startPeriod":"2026-10-01","total":-50}`)
	var got reconcileResult
	runNovelJSON(t, path, &got, "reconcile")
	if len(got.BilledWithoutActiveSubscription) != 0 {
		t.Fatalf("a refund was reported as billing after cancellation: %+v", got.BilledWithoutActiveSubscription)
	}
}

// Same class, other direction: an active subscription whose only line is a
// credit was not billed, so it is reported as missing its invoice line.
func TestR2CreditOnlyLineDoesNotCountAsBilled(t *testing.T) {
	path, db := novelStore(t)
	seedAsSync(t, db, "subscriptions", "", "",
		`{"id":"`+sidA+`","companyId":"`+cid1+`","productId":"`+pid1+`","status":"Active","billingTerm":"Monthly","quantity":1,"price":5}`)
	seedAsSync(t, db, "invoices", "", "", `{"id":"55555555-5555-4555-8555-555555555555","companyId":"`+cid1+`","total":-5}`)
	seedAsSync(t, db, "items", "invoices", "55555555-5555-4555-8555-555555555555",
		`{"id":"44444444-4444-4444-8444-444444444442","type":"rebate","companyId":"`+cid1+`","subscriptionId":"`+sidA+`","startPeriod":"2026-10-01","total":-5}`)
	var got reconcileResult
	runNovelJSON(t, path, &got, "reconcile")
	if len(got.ActiveWithoutInvoice) != 1 {
		t.Fatalf("a credit-only subscription counted as billed: %+v", got)
	}
}
