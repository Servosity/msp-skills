// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written (Phase 3): shared store-seeding helper for novel-feature tests.

package cli

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"pax8-pp-cli/internal/store"
)

// seedTranscendenceStore creates a temp SQLite store populated with a small but
// realistic fixture exercising every novel command:
//   - c-active:    active subscription + invoice           (fully reconciled)
//   - c-billed:    invoice but only a Cancelled sub        (billed_without_active_subscription)
//   - c-noinvoice: active subscription but NO invoice      (active_without_invoice)
//
// The store's Upsert* methods each take a SINGLE object and extract the id from
// it, so the helper inserts one record per call. Fixtures use the camelCase keys
// the store's typed-column extractors read.
func seedTranscendenceStore(t *testing.T) *store.Store {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "pax8-test.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}

	type rec struct {
		label string
		fn    func(json.RawMessage) error
		json  string
	}
	recs := []rec{
		{"company c-active", db.UpsertCompanies, `{"id":"c-active","name":"Active MSP","status":"Active"}`},
		{"company c-billed", db.UpsertCompanies, `{"id":"c-billed","name":"Billed MSP","status":"Active"}`},
		{"company c-noinvoice", db.UpsertCompanies, `{"id":"c-noinvoice","name":"NoInvoice MSP","status":"Active"}`},

		{"product p-m365", db.UpsertProducts, `{"id":"p-m365","name":"Microsoft 365 Business","vendorName":"Microsoft","sku":"M365-BP"}`},
		{"product p-azure", db.UpsertProducts, `{"id":"p-azure","name":"Azure Plan","vendorName":"Microsoft","sku":"AZ-PLAN"}`},

		{"sub s-active", db.UpsertSubscriptions, `{"id":"s-active","companyId":"c-active","productId":"p-m365","status":"Active","price":20.0,"quantity":5,"partnerCost":15.0,"currencyCode":"USD","createdDate":"2026-05-20T00:00:00Z"}`},
		{"sub s-noinv", db.UpsertSubscriptions, `{"id":"s-noinv","companyId":"c-noinvoice","productId":"p-azure","status":"Active","price":100.0,"quantity":1,"partnerCost":80.0,"currencyCode":"USD","createdDate":"2026-05-27T00:00:00Z"}`},
		{"sub s-cancelled", db.UpsertSubscriptions, `{"id":"s-cancelled","companyId":"c-billed","productId":"p-m365","status":"Cancelled","price":20.0,"quantity":2,"partnerCost":15.0,"currencyCode":"USD","createdDate":"2026-01-01T00:00:00Z"}`},

		{"invoice i-active", db.UpsertInvoices, `{"id":"i-active","companyId":"c-active","total":100.0,"currencyCode":"USD","invoiceDate":"2026-05-01"}`},
		{"invoice i-billed", db.UpsertInvoices, `{"id":"i-billed","companyId":"c-billed","total":250.0,"currencyCode":"USD","invoiceDate":"2026-05-01"}`},

		{"usage u-normal", db.UpsertUsageSummaries, `{"id":"u-normal","companyId":"c-active","productId":"p-azure","currentCharges":10.0,"vendorName":"Microsoft"}`},
		{"usage u-normal2", db.UpsertUsageSummaries, `{"id":"u-normal2","companyId":"c-active","productId":"p-azure","currentCharges":12.0,"vendorName":"Microsoft"}`},
		{"usage u-spike", db.UpsertUsageSummaries, `{"id":"u-spike","companyId":"c-noinvoice","productId":"p-azure","currentCharges":500.0,"vendorName":"Microsoft"}`},
	}
	for _, r := range recs {
		if err := r.fn(json.RawMessage(r.json)); err != nil {
			db.Close()
			t.Fatalf("seed %s: %v", r.label, err)
		}
	}
	return db
}
