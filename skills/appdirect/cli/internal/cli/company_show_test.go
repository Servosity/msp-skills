// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored behavioral tests for the company show novel feature.

package cli

import (
	"testing"
	"time"
)

func TestCompanyShowJoinsAllEntities(t *testing.T) {
	db, path := newNovelTestDB(t)
	now := time.Now().UTC()

	seedNovelResource(t, db, rtCompanies, "c1", map[string]any{"uuid": "c1", "name": "Acme MSP", "enabled": true})
	seedNovelResource(t, db, rtCompanies, "c2", map[string]any{"uuid": "c2", "name": "Globex"})

	seedNovelResource(t, db, rtUsers, "u1", map[string]any{
		"id": "u1", "email": "admin@acme.example", "firstName": "Ada", "lastName": "Admin",
		"company": map[string]any{"id": "c1"},
	})
	seedNovelResource(t, db, rtUsers, "u2", map[string]any{
		"id": "u2", "email": "other@globex.example",
		"company": map[string]any{"id": "c2"},
	})

	seedNovelResource(t, db, rtSubscriptions, "s1", map[string]any{
		"id": "s1", "status": "ACTIVE",
		"company": map[string]any{"id": "c1"},
		"product": map[string]any{"id": "prod1", "name": "M365 Business"},
	})
	seedNovelResource(t, db, rtInvoices, "i1", map[string]any{
		"invoiceId": 7, "status": "UNPAID", "total": 120.0, "currency": "USD",
		"company": map[string]any{"id": "c1"},
		"dueDate": epochMS(now.Add(5 * 24 * time.Hour)),
	})
	seedNovelResource(t, db, rtOpportunities, "o1", map[string]any{
		"id": "o1", "name": "Upsell backup", "status": "OPEN",
		"ownerUser":    map[string]any{"email": "sofia@reseller.example"},
		"customerUser": map[string]any{"id": "cu1", "company": map[string]any{"id": "c1"}},
		"createdOn":    epochMS(now.Add(-3 * 24 * time.Hour)),
	})
	// CLOSED opportunity for the same company: excluded (open-only view).
	seedNovelResource(t, db, rtOpportunities, "o2", map[string]any{
		"id": "o2", "status": "CLOSED",
		"customerUser": map[string]any{"company": map[string]any{"id": "c1"}},
	})

	out, err := runNovelCmd(t, newNovelCompanyShowCmd, "c1", "--db", path)
	if err != nil {
		t.Fatalf("company show: %v\n%s", err, out)
	}
	view := decodeNovelJSON(t, out)

	companyObj, _ := view["company"].(map[string]any)
	if companyObj == nil || companyObj["name"] != "Acme MSP" {
		t.Fatalf("company = %v, want Acme MSP", view["company"])
	}
	counts := view["counts"].(map[string]any)
	for k, want := range map[string]float64{"users": 1, "subscriptions": 1, "invoices": 1, "opportunities": 1} {
		if counts[k].(float64) != want {
			t.Fatalf("counts[%s] = %v, want %v\n%s", k, counts[k], want, out)
		}
	}
	users := novelList(t, view, "users")
	if users[0].(map[string]any)["email"] != "admin@acme.example" {
		t.Fatalf("user join leaked wrong company's user: %v", users)
	}
	subs := novelList(t, view, "subscriptions")
	if subs[0].(map[string]any)["product"] != "M365 Business" {
		t.Fatalf("subscription product name missing: %v", subs)
	}
}

func TestCompanyShowUnknownCompanyHasNote(t *testing.T) {
	db, path := newNovelTestDB(t)
	seedNovelResource(t, db, rtCompanies, "c1", map[string]any{"uuid": "c1", "name": "Acme MSP"})
	out, err := runNovelCmd(t, newNovelCompanyShowCmd, "nope", "--db", path)
	if err != nil {
		t.Fatalf("company show: %v", err)
	}
	view := decodeNovelJSON(t, out)
	if note, _ := view["note"].(string); note == "" {
		t.Fatal("expected not-found note for unknown company")
	}
}

func TestCompanyShowMissingArgIsUsageError(t *testing.T) {
	_, path := newNovelTestDB(t)
	_, err := runNovelCmd(t, newNovelCompanyShowCmd, "--db", path)
	if err == nil {
		t.Fatal("expected usage error when companyId is missing")
	}
}
