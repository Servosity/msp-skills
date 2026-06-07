// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored tests for the Kaseya BMS novel commands.

package cli

import (
	"testing"
	"time"
)

var wlNow = time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC)

func TestAggregateWorkload(t *testing.T) {
	tickets := []map[string]any{
		{"AssigneeName": "Jane Smith", "PriorityName": "High", "StatusName": "New"},
		{"AssigneeName": "Jane Smith", "PriorityName": "Low", "StatusName": "In Progress"},
		{"AssigneeName": "Bob Lee", "PriorityName": "High", "StatusName": "New"},
		{"AssigneeName": "", "PriorityName": "High", "StatusName": "New"},
		{"AssigneeName": "Jane Smith", "StatusName": "Completed", "CompletedDate": "2026-06-01T10:00:00"},
	}
	timelogs := []map[string]any{
		{"FirstName": "Jane", "LastName": "Smith", "Timespent": float64(120), "StartDate": "2026-06-05T09:00:00"},
		{"FirstName": "Jane", "LastName": "Smith", "Timespent": float64(60), "StartDate": "2026-06-06T09:00:00"},
		{"FirstName": "Jane", "LastName": "Smith", "Timespent": float64(60), "StartDate": "2026-01-01T09:00:00"}, // outside window
		{"FirstName": "Bob", "LastName": "Lee", "Timespent": "90", "StartDate": "2026-06-05T09:00:00"},           // numeric string
	}
	got := aggregateWorkload(tickets, timelogs, 7, "minutes", wlNow)
	if got.TotalOpen != 4 {
		t.Errorf("TotalOpen = %d, want 4", got.TotalOpen)
	}
	if got.Unassigned != 1 {
		t.Errorf("Unassigned = %d, want 1", got.Unassigned)
	}
	if len(got.Items) != 2 {
		t.Fatalf("len(Items) = %d, want 2", len(got.Items))
	}
	if got.Items[0].Assignee != "Jane Smith" || got.Items[0].OpenTickets != 2 {
		t.Errorf("top item = %+v, want Jane Smith with 2 open", got.Items[0])
	}
	if got.Items[0].HoursLogged != 3.0 {
		t.Errorf("Jane hours = %v, want 3.0 (window-filtered, minutes->hours)", got.Items[0].HoursLogged)
	}
	if got.Items[1].HoursLogged != 1.5 {
		t.Errorf("Bob hours = %v, want 1.5 (numeric-string Timespent)", got.Items[1].HoursLogged)
	}
}

func TestAggregateWorkloadIDJoin(t *testing.T) {
	// AssigneeName disagrees with FirstName+LastName; the numeric ID must join them.
	tickets := []map[string]any{
		{"AssigneeName": "Smith, Jane", "AssigneeId": float64(7), "PriorityName": "High", "StatusName": "New"},
	}
	timelogs := []map[string]any{
		{"FirstName": "Jane", "LastName": "Smith", "UserId": float64(7), "Timespent": float64(60), "StartDate": "2026-06-05T09:00:00"},
	}
	got := aggregateWorkload(tickets, timelogs, 7, "minutes", wlNow)
	if len(got.Items) != 1 {
		t.Fatalf("len(Items) = %d, want 1 (ID join must merge the rows)", len(got.Items))
	}
	if got.Items[0].OpenTickets != 1 || got.Items[0].HoursLogged != 1.0 {
		t.Errorf("merged row = %+v, want 1 open + 1.0h", got.Items[0])
	}
}

func TestAggregateWorkloadAvgDenominator(t *testing.T) {
	// A tech with hours but no open tickets must not dilute avg_open_per_tech.
	tickets := []map[string]any{
		{"AssigneeName": "Jane Smith", "AssigneeId": float64(1), "PriorityName": "High", "StatusName": "New"},
		{"AssigneeName": "Jane Smith", "AssigneeId": float64(1), "PriorityName": "Low", "StatusName": "New"},
	}
	timelogs := []map[string]any{
		{"FirstName": "Bob", "LastName": "Lee", "UserId": float64(2), "Timespent": float64(60), "StartDate": "2026-06-05T09:00:00"},
	}
	got := aggregateWorkload(tickets, timelogs, 7, "minutes", wlNow)
	if got.AvgOpenPerTech != 2.0 {
		t.Errorf("AvgOpenPerTech = %v, want 2.0 (denominator excludes hours-only techs)", got.AvgOpenPerTech)
	}
}

func TestAggregateWorkloadEmpty(t *testing.T) {
	got := aggregateWorkload(nil, nil, 7, "minutes", wlNow)
	if got.Note == "" {
		t.Errorf("expected note on empty mirror")
	}
}
