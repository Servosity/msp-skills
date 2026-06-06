// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestCustomersProfileCard(t *testing.T) {
	db, dbPath := newNovelTestStore(t)

	seedResource(t, db, "customers", "42", map[string]any{
		"business_then_name": "Acme Corp",
	})
	seedResource(t, db, "tickets", "t1", map[string]any{
		"customer_id": "42", "status": "New", "subject": "Server down", "number": "1001",
		"created_at": "2026-06-01T10:00:00Z",
	})
	seedResource(t, db, "tickets", "t2", map[string]any{
		"customer_id": "42", "status": "Resolved", "subject": "Old issue", "number": "1000",
		"created_at": "2026-05-01T10:00:00Z",
	})
	seedResource(t, db, "tickets", "t3", map[string]any{
		"customer_id": "99", "status": "New", "subject": "Other customer", "number": "1002",
	})
	seedCustomerAsset(t, db, "a1", map[string]any{"customer_id": "42", "name": "WS-01"})
	seedCustomerAsset(t, db, "a2", map[string]any{"customer_id": "42", "name": "WS-02"})
	seedCustomerAsset(t, db, "a3", map[string]any{"customer_id": "99", "name": "WS-03"})
	if _, err := db.DB().Exec(
		`INSERT INTO invoices(id, is_paid, total, data) VALUES(?, ?, ?, ?)`,
		"i1", 0, 150.50, `{"customer_id":"42"}`,
	); err != nil {
		t.Fatalf("seed invoice: %v", err)
	}
	if _, err := db.DB().Exec(
		`INSERT INTO invoices(id, is_paid, total, data) VALUES(?, ?, ?, ?)`,
		"i2", 1, 999.99, `{"customer_id":"42"}`,
	); err != nil {
		t.Fatalf("seed paid invoice: %v", err)
	}
	seedResource(t, db, "contracts", "c1", map[string]any{
		"customer_id": "42", "name": "Gold MSA",
	})
	seedResource(t, db, "rmm_alerts", "al1", map[string]any{
		"customer_id": "42", "created_at": "2026-06-05T08:00:00Z",
	})
	db.Close()

	flags := &rootFlags{asJSON: true}
	cmd := newNovelCustomersProfileCmd(flags)
	out, _ := execNovel(t, cmd, []string{"42", "--db", dbPath, "--alert-window", "3650d"})

	if got := out["customer_name"]; got != "Acme Corp" {
		t.Fatalf("customer_name = %v, want Acme Corp", got)
	}
	if got := jsonFloat(t, out, "ticket_total"); got != 2 {
		t.Fatalf("ticket_total = %v, want 2", got)
	}
	if got := jsonFloat(t, out, "ticket_open"); got != 1 {
		t.Fatalf("ticket_open = %v, want 1", got)
	}
	if got := jsonFloat(t, out, "asset_count"); got != 2 {
		t.Fatalf("asset_count = %v, want 2", got)
	}
	if got := jsonFloat(t, out, "unpaid_invoice_total"); got != 150.50 {
		t.Fatalf("unpaid_invoice_total = %v, want 150.50", got)
	}
	if got := jsonFloat(t, out, "unpaid_invoice_count"); got != 1 {
		t.Fatalf("unpaid_invoice_count = %v, want 1", got)
	}
	if got := jsonFloat(t, out, "contract_count"); got != 1 {
		t.Fatalf("contract_count = %v, want 1", got)
	}
	if got := jsonFloat(t, out, "alert_count_window"); got != 1 {
		t.Fatalf("alert_count_window = %v, want 1", got)
	}
	openTickets, ok := out["open_tickets"].([]any)
	if !ok || len(openTickets) != 1 {
		t.Fatalf("open_tickets = %v, want exactly 1 entry", out["open_tickets"])
	}
	first, _ := openTickets[0].(map[string]any)
	if first["subject"] != "Server down" {
		t.Fatalf("open ticket subject = %v, want Server down", first["subject"])
	}
}

func TestCustomersProfileEmptyStoreZeroValues(t *testing.T) {
	db, dbPath := newNovelTestStore(t)
	db.Close()

	flags := &rootFlags{asJSON: true}
	cmd := newNovelCustomersProfileCmd(flags)
	out, _ := execNovel(t, cmd, []string{"42", "--db", dbPath})

	if got := out["customer_synced"]; got != false {
		t.Fatalf("customer_synced = %v, want false", got)
	}
	if got := jsonFloat(t, out, "ticket_total"); got != 0 {
		t.Fatalf("ticket_total = %v, want 0", got)
	}
	// Zero-vs-missing contract: empty aggregates must be [] not null.
	if _, ok := out["open_tickets"].([]any); !ok {
		t.Fatalf("open_tickets must be a JSON array, got %T", out["open_tickets"])
	}
	if _, ok := out["contracts"].([]any); !ok {
		t.Fatalf("contracts must be a JSON array, got %T", out["contracts"])
	}
}

func TestCustomersProfileMissingArgUsageError(t *testing.T) {
	flags := &rootFlags{asJSON: true}
	cmd := newNovelCustomersProfileCmd(flags)
	cmd.SetArgs([]string{"--db", "/nonexistent/never-opened.db"})
	cmd.SetOut(&strings.Builder{})
	cmd.SetErr(&strings.Builder{})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected usage error for missing customer-id, got nil")
	}
	if !strings.Contains(err.Error(), "customer-id is required") {
		t.Fatalf("err = %v, want customer-id required", err)
	}
}

func TestCustomersProfileSelectAndCaps(t *testing.T) {
	db, dbPath := newNovelTestStore(t)
	seedResource(t, db, "customers", "42", map[string]any{"name": "Acme"})
	// 7 open tickets to exercise the newest-5 truncation.
	for i := 0; i < 7; i++ {
		seedResource(t, db, "tickets", fmt.Sprintf("t%d", i), map[string]any{
			"customer_id": "42", "status": "New",
			"subject":    fmt.Sprintf("Ticket %d", i),
			"number":     fmt.Sprintf("10%02d", i),
			"created_at": fmt.Sprintf("2026-06-0%dT10:00:00Z", i+1),
		})
	}
	// One in-window and one out-of-window alert pin the cutoff filter and
	// the latest-of-multiple selection.
	seedResource(t, db, "rmm_alerts", "al-old", map[string]any{
		"customer_id": "42", "created_at": "2020-01-01T00:00:00Z",
	})
	seedResource(t, db, "rmm_alerts", "al-new", map[string]any{
		"customer_id": "42", "created_at": time.Now().UTC().Add(-time.Hour).Format(time.RFC3339),
	})
	db.Close()

	flags := &rootFlags{asJSON: true}
	cmd := newNovelCustomersProfileCmd(flags)
	out, _ := execNovel(t, cmd, []string{"42", "--db", dbPath, "--alert-window", "7d"})

	openTickets, _ := out["open_tickets"].([]any)
	if len(openTickets) != 5 {
		t.Fatalf("open_tickets len = %d, want 5 (truncated from 7)", len(openTickets))
	}
	first, _ := openTickets[0].(map[string]any)
	if first["subject"] != "Ticket 6" {
		t.Fatalf("newest open ticket = %v, want Ticket 6", first["subject"])
	}
	if got := jsonFloat(t, out, "alert_count_window"); got != 1 {
		t.Fatalf("alert_count_window = %v, want 1 (out-of-window alert excluded)", got)
	}
	if got, _ := out["latest_alert_at"].(string); got == "" || got == "2020-01-01T00:00:00Z" {
		t.Fatalf("latest_alert_at = %q, want the in-window alert timestamp", got)
	}

	// --select must narrow the card to exactly the requested keys, and an
	// empty aggregate selected through it must stay [] (never null/omitted).
	flags2 := &rootFlags{asJSON: true, selectFields: "customer_id,unpaid_invoice_total,contracts"}
	cmd2 := newNovelCustomersProfileCmd(flags2)
	out2, _ := execNovel(t, cmd2, []string{"42", "--db", dbPath})
	if len(out2) != 3 {
		t.Fatalf("--select returned %d keys (%v), want exactly 3", len(out2), out2)
	}
	if _, ok := out2["contracts"].([]any); !ok {
		t.Fatalf("--select contracts = %T (%v), want [] JSON array", out2["contracts"], out2["contracts"])
	}
}
