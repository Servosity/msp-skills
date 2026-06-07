// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored behavioral tests for the payments unpaid novel feature.

package cli

import (
	"testing"
	"time"
)

func TestPaymentsUnpaidFiltersAndSorts(t *testing.T) {
	db, path := newNovelTestDB(t)
	now := time.Now().UTC()

	seedNovelResource(t, db, rtCompanies, "c1", map[string]any{"uuid": "c1", "name": "Acme MSP"})
	seedNovelResource(t, db, rtPayments, "p1", map[string]any{
		"id": 1, "result": "FAILED", "amount": 50.0, "currency": "USD",
		"company": map[string]any{"id": "c1"},
		"date":    epochMS(now.Add(-24 * time.Hour)),
	})
	seedNovelResource(t, db, rtPayments, "p2", map[string]any{
		"id": 2, "result": "GATEWAY_NOT_AVAILABLE", "amount": 500.0, "currency": "USD",
		"company": map[string]any{"id": "c1"},
		"date":    epochMS(now.Add(-48 * time.Hour)),
	})
	// Negative rows: successful, and failed-but-outside-window.
	seedNovelResource(t, db, rtPayments, "p3", map[string]any{
		"id": 3, "result": "SUCCESSFUL", "amount": 999.0,
		"company": map[string]any{"id": "c1"},
		"date":    epochMS(now.Add(-24 * time.Hour)),
	})
	seedNovelResource(t, db, rtPayments, "p4", map[string]any{
		"id": 4, "result": "FAILED", "amount": 75.0,
		"company": map[string]any{"id": "c1"},
		"date":    epochMS(now.Add(-90 * 24 * time.Hour)),
	})

	out, err := runNovelCmd(t, newNovelPaymentsUnpaidCmd, "--since", "7d", "--db", path)
	if err != nil {
		t.Fatalf("payments unpaid: %v\n%s", err, out)
	}
	view := decodeNovelJSON(t, out)
	payments := novelList(t, view, "payments")
	if len(payments) != 2 {
		t.Fatalf("payments = %d, want 2 (FAILED + GATEWAY_NOT_AVAILABLE in window)\n%s", len(payments), out)
	}
	first := payments[0].(map[string]any)
	if first["paymentId"] != "2" {
		t.Fatalf("expected largest amount (500, id 2) first, got %v", first)
	}
	if first["companyName"] != "Acme MSP" {
		t.Fatalf("expected company-name join, got %v", first)
	}
	if view["scanned_payments"].(float64) != 4 {
		t.Fatalf("scanned_payments = %v, want 4", view["scanned_payments"])
	}
}

func TestPaymentsUnpaidEmptyHasNote(t *testing.T) {
	db, path := newNovelTestDB(t)
	seedNovelResource(t, db, rtPayments, "p1", map[string]any{
		"id": 1, "result": "SUCCESSFUL", "amount": 10.0,
		"date": epochMS(time.Now().UTC()),
	})
	out, err := runNovelCmd(t, newNovelPaymentsUnpaidCmd, "--since", "7d", "--db", path)
	if err != nil {
		t.Fatalf("payments unpaid: %v", err)
	}
	view := decodeNovelJSON(t, out)
	if n := len(novelList(t, view, "payments")); n != 0 {
		t.Fatalf("payments = %d, want 0", n)
	}
	if note, _ := view["note"].(string); note == "" {
		t.Fatal("expected honest empty-result note")
	}
}
