// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored behavioral tests for the reconcile novel feature.

package cli

import (
	"strings"
	"testing"
	"time"
)

func TestReconcileFindsMismatches(t *testing.T) {
	db, path := newNovelTestDB(t)
	now := time.Now().UTC()

	seedNovelResource(t, db, rtCompanies, "c1", map[string]any{"uuid": "c1", "name": "Acme MSP"})
	seedNovelResource(t, db, rtCompanies, "c2", map[string]any{"uuid": "c2", "name": "Globex"})

	// Invoiced ACTIVE subscription: order o1 appears on a recent invoice.
	seedNovelResource(t, db, rtSubscriptions, "s1", map[string]any{
		"id": "s1", "status": "ACTIVE",
		"company": map[string]any{"id": "c1"},
		"order":   map[string]any{"id": "o1"},
	})
	// Uninvoiced ACTIVE subscription: no invoice references order o2.
	seedNovelResource(t, db, rtSubscriptions, "s2", map[string]any{
		"id": "s2", "status": "ACTIVE",
		"company": map[string]any{"id": "c2"},
		"order":   map[string]any{"id": "o2"},
	})
	// CANCELLED subscription must not be flagged even with no invoice.
	seedNovelResource(t, db, rtSubscriptions, "s3", map[string]any{
		"id": "s3", "status": "CANCELLED",
		"company": map[string]any{"id": "c2"},
		"order":   map[string]any{"id": "o3"},
	})

	// Recent PAID invoice covering o1.
	seedNovelResource(t, db, rtInvoices, "i1", map[string]any{
		"invoiceId": 1, "status": "PAID",
		"company":      map[string]any{"id": "c1"},
		"orderIds":     []any{"o1"},
		"creationDate": epochMS(now.Add(-24 * time.Hour)),
		"dueDate":      epochMS(now.Add(13 * 24 * time.Hour)),
		"total":        99.0, "currency": "USD",
	})
	// Overdue UNPAID invoice.
	seedNovelResource(t, db, rtInvoices, "i2", map[string]any{
		"invoiceId": 2, "status": "UNPAID",
		"company":      map[string]any{"id": "c2"},
		"orderIds":     []any{"o9"},
		"creationDate": epochMS(now.Add(-40 * 24 * time.Hour)),
		"dueDate":      epochMS(now.Add(-10 * 24 * time.Hour)),
		"total":        250.0, "currency": "USD",
	})

	// Recent FAILED payment plus an old SUCCESSFUL one (must be excluded).
	seedNovelResource(t, db, rtPayments, "p1", map[string]any{
		"id": 11, "result": "FAILED", "amount": 42.5, "currency": "USD",
		"company": map[string]any{"id": "c1"},
		"date":    epochMS(now.Add(-2 * 24 * time.Hour)),
	})
	seedNovelResource(t, db, rtPayments, "p2", map[string]any{
		"id": 12, "result": "SUCCESSFUL", "amount": 10.0, "currency": "USD",
		"company": map[string]any{"id": "c1"},
		"date":    epochMS(now.Add(-2 * 24 * time.Hour)),
	})

	out, err := runNovelCmd(t, newNovelReconcileCmd, "--since", "30d", "--db", path)
	if err != nil {
		t.Fatalf("reconcile: %v\n%s", err, out)
	}
	view := decodeNovelJSON(t, out)
	findings := novelList(t, view, "findings")

	kinds := map[string][]string{}
	for _, f := range findings {
		fm := f.(map[string]any)
		kind := fm["kind"].(string)
		id, _ := fm["subscriptionId"].(string)
		if id == "" {
			if v, ok := fm["invoiceId"].(string); ok {
				id = v
			}
		}
		if id == "" {
			if v, ok := fm["paymentId"].(string); ok {
				id = v
			}
		}
		kinds[kind] = append(kinds[kind], id)
	}

	if got := kinds["subscription-without-invoice"]; len(got) != 1 || got[0] != "s2" {
		t.Fatalf("subscription-without-invoice = %v, want exactly [s2]\noutput: %s", got, out)
	}
	if got := kinds["overdue-invoice"]; len(got) != 1 || got[0] != "2" {
		t.Fatalf("overdue-invoice = %v, want exactly [2]\noutput: %s", got, out)
	}
	if got := kinds["failed-payment"]; len(got) != 1 || got[0] != "11" {
		t.Fatalf("failed-payment = %v, want exactly [11]\noutput: %s", got, out)
	}
	// Company-name join must resolve from the synced companies table.
	if !strings.Contains(out, "Globex") {
		t.Fatalf("expected company-name join (Globex) in output: %s", out)
	}
}

func TestReconcileCompanyFilterAndCleanState(t *testing.T) {
	db, path := newNovelTestDB(t)
	now := time.Now().UTC()
	seedNovelResource(t, db, rtSubscriptions, "s1", map[string]any{
		"id": "s1", "status": "ACTIVE",
		"company": map[string]any{"id": "c1"},
		"order":   map[string]any{"id": "o1"},
	})
	seedNovelResource(t, db, rtInvoices, "i1", map[string]any{
		"invoiceId": 1, "status": "PAID",
		"company":      map[string]any{"id": "c1"},
		"orderIds":     []any{"o1"},
		"creationDate": epochMS(now.Add(-24 * time.Hour)),
	})

	// Filtering to a different company yields zero findings plus a note.
	out, err := runNovelCmd(t, newNovelReconcileCmd, "--company", "c9", "--db", path)
	if err != nil {
		t.Fatalf("reconcile --company: %v", err)
	}
	view := decodeNovelJSON(t, out)
	if n := len(novelList(t, view, "findings")); n != 0 {
		t.Fatalf("findings for unmatched company = %d, want 0", n)
	}
	if note, _ := view["note"].(string); note == "" {
		t.Fatalf("expected honest empty-result note, got none: %s", out)
	}
}

func TestReconcileInvoiceWithoutCreationDateStillCovers(t *testing.T) {
	db, path := newNovelTestDB(t)
	// ACTIVE sub whose order IS invoiced, but the invoice has no creationDate.
	seedNovelResource(t, db, rtSubscriptions, "s1", map[string]any{
		"id": "s1", "status": "ACTIVE",
		"company": map[string]any{"id": "c1"},
		"order":   map[string]any{"id": "o1"},
	})
	seedNovelResource(t, db, rtInvoices, "i1", map[string]any{
		"invoiceId": 1, "status": "PAID",
		"company":  map[string]any{"id": "c1"},
		"orderIds": []any{"o1"},
	})
	out, err := runNovelCmd(t, newNovelReconcileCmd, "--since", "30d", "--db", path)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	view := decodeNovelJSON(t, out)
	for _, f := range novelList(t, view, "findings") {
		if f.(map[string]any)["kind"] == "subscription-without-invoice" {
			t.Fatalf("dateless invoice must still cover its order; got false flag: %s", out)
		}
	}
}

func TestReconcileBadSinceIsUsageError(t *testing.T) {
	_, path := newNovelTestDB(t)
	_, err := runNovelCmd(t, newNovelReconcileCmd, "--since", "tomorrow", "--db", path)
	if err == nil {
		t.Fatal("expected usage error for --since tomorrow")
	}
}
