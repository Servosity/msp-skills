// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored tests for the Kaseya BMS novel commands.

package cli

import "testing"

func ubLog(account string, minutes float64, billable, billed, approved bool, start string) map[string]any {
	return map[string]any{
		"AccountName": account, "Timespent": minutes,
		"IsBillable": billable, "IsBilled": billed, "IsApproved": approved,
		"StartDate": start,
	}
}

func TestAggregateUnbilled(t *testing.T) {
	rows := []map[string]any{
		ubLog("Acme Corp", 120, true, false, true, "2026-05-01T09:00:00"),
		ubLog("Acme Corp", 60, true, false, true, "2026-04-01T09:00:00"),
		ubLog("Acme Corp", 60, true, true, true, "2026-05-01T09:00:00"),  // already billed
		ubLog("Beta LLC", 90, true, false, false, "2026-05-02T09:00:00"), // not approved
		ubLog("Beta LLC", 30, false, false, true, "2026-05-02T09:00:00"), // not billable
	}
	got := aggregateUnbilled(rows, "minutes", true)
	if len(got.Items) != 1 {
		t.Fatalf("len(Items) = %d, want 1 (approved only)", len(got.Items))
	}
	if got.Items[0].Account != "Acme Corp" || got.Items[0].Hours != 3.0 || got.Items[0].Entries != 2 {
		t.Errorf("item = %+v, want Acme Corp 3.0h 2 entries", got.Items[0])
	}
	if got.Items[0].OldestStart != "2026-04-01" {
		t.Errorf("OldestStart = %s, want 2026-04-01", got.Items[0].OldestStart)
	}

	withUnapproved := aggregateUnbilled(rows, "minutes", false)
	if len(withUnapproved.Items) != 2 || withUnapproved.TotalHours != 4.5 {
		t.Errorf("include-unapproved: items=%d total=%v, want 2 items 4.5h", len(withUnapproved.Items), withUnapproved.TotalHours)
	}
}

func TestAggregateUnbilledEmpty(t *testing.T) {
	got := aggregateUnbilled(nil, "minutes", true)
	if got.Note == "" {
		t.Errorf("expected note on empty result")
	}
}
