// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// Codex review r1 classes (kaseya-bms 4.34.2 reprint), each over a documented BMS shape:
//   - terminal-status classification (TicketListDto.StatusName): a status is terminal only as a whole word;
//   - record identity (TicketListDto.AssigneeId, ClientContractSummaryOutputDto.Id / TicketListDto.ContractId):
//     two records with different known ids are never merged by display name;
//   - incremental filter on a creation or start date (Filter.OpenDateFrom, Filter.CreatedOnFrom,
//     Filter.StartDateFrom): such a filter never sees a later change, so no partition syncs incrementally on one.

import (
	"testing"
	"time"
)

func TestTicketStatusIsTerminalOnlyAsAWholeWord(t *testing.T) {
	for status, open := range map[string]bool{
		"Unresolved": true, "Not Resolved": true, "Not Completed": true, "Pending Closure": true, "Awaiting Close": true,
		"In Progress": true, "New": true, "Reopened": true,
		"Resolved": false, "Completed": false, "Closed": false, "Closed - Billing": false, "Cancelled": false, "canceled": false,
		// Codex r2: a negating word elsewhere in the status describes something else.
		"Cancelled - No Charge": false, "Resolved - Not Billed": false, "Closed (no response)": false, "Completed, pending invoice": false,
	} {
		if got := kbmsTicketOpen(map[string]any{"StatusName": status}); got != open {
			t.Errorf("StatusName %q: open = %v, want %v", status, got, open)
		}
	}
	if kbmsTicketOpen(map[string]any{"StatusName": "Unresolved", "CompletedDate": "2026-10-01T00:00:00Z"}) {
		t.Error("a ticket with a CompletedDate is not open, whatever its status reads")
	}
}

func TestWorkloadNeverMergesTwoKnownTechsByName(t *testing.T) {
	now := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	tickets := []map[string]any{
		{"Id": float64(1), "AssigneeId": float64(7), "AssigneeName": "Alex Smith", "StatusName": "New"},
		{"Id": float64(2), "AssigneeId": float64(8), "AssigneeName": "Alex Smith", "StatusName": "New"},
		// One tech whose name is spelled two ways still joins on the id.
		{"Id": float64(3), "AssigneeId": float64(9), "AssigneeName": "Smith, Jane", "StatusName": "New"},
		{"Id": float64(4), "AssigneeId": float64(9), "AssigneeName": "Jane Smith", "StatusName": "New"},
	}
	v := aggregateWorkload(tickets, nil, 7, "minutes", now)
	if len(v.Items) != 3 {
		t.Fatalf("want 3 technicians (two named Alex Smith, one Jane), got %d: %+v", len(v.Items), v.Items)
	}
	for _, it := range v.Items {
		if it.Assignee == "Alex Smith" && it.OpenTickets != 1 {
			t.Errorf("Alex Smith row carries %d tickets; two techs with one name were merged", it.OpenTickets)
		}
	}
	if v.AvgOpenPerTech != float64(4)/3 && v.AvgOpenPerTech != 1.33 {
		t.Errorf("avg_open_per_tech = %v, want 4/3", v.AvgOpenPerTech)
	}
}

func TestContractBurnKeysContractsByID(t *testing.T) {
	now := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	contracts := []map[string]any{
		{"Id": float64(1), "ContractName": "Support", "AccountName": "Acme"},
		{"Id": float64(2), "ContractName": "Support", "AccountName": "Beta"},
	}
	tickets := []map[string]any{
		{"Id": float64(11), "ContractId": float64(1), "ContractName": "Support", "AccountName": "Acme", "StatusName": "New"},
		{"Id": float64(12), "ContractId": float64(2), "ContractName": "Support", "AccountName": "Beta", "StatusName": "New"},
		{"Id": float64(13), "ContractId": float64(2), "ContractName": "Support", "AccountName": "Beta", "StatusName": "New"},
	}
	logs := []map[string]any{
		{"TicketId": float64(11), "Timespent": float64(60), "StartDate": "2026-10-28T09:00:00Z"},
		{"TicketId": float64(12), "Timespent": float64(120), "StartDate": "2026-10-28T09:00:00Z"},
	}
	v := aggregateContractBurn(contracts, tickets, logs, 30, "minutes", now)
	if len(v.Items) != 2 {
		t.Fatalf("want 2 contracts (same name, different ids), got %d: %+v", len(v.Items), v.Items)
	}
	got := map[string]contractBurnItem{}
	for _, it := range v.Items {
		got[it.Account] = it
	}
	if a, b := got["Acme"], got["Beta"]; a.HoursConsumed != 1 || a.OpenTickets != 1 || b.HoursConsumed != 2 || b.OpenTickets != 2 {
		t.Fatalf("contracts merged or misrouted: Acme %+v, Beta %+v", a, b)
	}
}

func TestNoPartitionSyncsIncrementallyOnACreationDate(t *testing.T) {
	for _, name := range knownSyncResourceNames() {
		if p := syncResourceSinceParam(name); p != "" {
			t.Errorf("%s syncs incrementally on %s, a creation/open/start date that never sees a later change", name, p)
		}
	}
}
