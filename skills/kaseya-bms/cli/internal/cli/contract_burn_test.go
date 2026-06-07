// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored tests for the Kaseya BMS novel commands.

package cli

import (
	"testing"
	"time"
)

var cbNow = time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC)

func TestAggregateContractBurn(t *testing.T) {
	contracts := []map[string]any{
		{"ContractName": "Gold MSP", "AccountName": "Acme Corp", "ContractType": "Recurring", "Status": "Active", "StartDate": "2026-01-01T00:00:00", "EndDate": "2026-12-31T00:00:00"},
		{"ContractName": "Silver MSP", "AccountName": "Beta LLC", "Status": "Active"},
	}
	tickets := []map[string]any{
		{"Id": float64(11), "ContractName": "Gold MSP", "AccountName": "Acme Corp", "StatusName": "New"},
		{"Id": float64(12), "ContractName": "Gold MSP", "AccountName": "Acme Corp", "StatusName": "Completed", "CompletedDate": "2026-06-01T00:00:00"},
	}
	timelogs := []map[string]any{
		{"TicketId": float64(11), "Timespent": float64(120), "StartDate": "2026-06-01T09:00:00"},
		{"TicketId": float64(12), "Timespent": float64(60), "StartDate": "2026-06-02T09:00:00"},
		{"TicketId": float64(99), "Timespent": float64(600), "StartDate": "2026-06-02T09:00:00"}, // unrouted ticket
	}
	got := aggregateContractBurn(contracts, tickets, timelogs, 90, "minutes", cbNow)
	if len(got.Items) != 2 {
		t.Fatalf("len(Items) = %d, want 2", len(got.Items))
	}
	top := got.Items[0]
	if top.Contract != "Gold MSP" {
		t.Fatalf("top contract = %s, want Gold MSP", top.Contract)
	}
	if top.HoursConsumed != 3.0 {
		t.Errorf("HoursConsumed = %v, want 3.0", top.HoursConsumed)
	}
	if top.OpenTickets != 1 {
		t.Errorf("OpenTickets = %d, want 1", top.OpenTickets)
	}
	if top.PercentPeriodElapsed == nil || *top.PercentPeriodElapsed < 40 || *top.PercentPeriodElapsed > 50 {
		t.Errorf("PercentPeriodElapsed = %v, want ~43", top.PercentPeriodElapsed)
	}
	if top.Account != "Acme Corp" {
		t.Errorf("Account = %s, want Acme Corp", top.Account)
	}
}

func TestAggregateContractBurnEmpty(t *testing.T) {
	got := aggregateContractBurn(nil, nil, nil, 90, "minutes", cbNow)
	if got.Note == "" {
		t.Errorf("expected note on empty mirror")
	}
}
