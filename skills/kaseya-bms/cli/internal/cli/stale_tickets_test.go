// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored tests for the Kaseya BMS novel commands.

package cli

import (
	"testing"
	"time"
)

var stNow = time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC)

func TestFilterStaleTickets(t *testing.T) {
	rows := []map[string]any{
		{"TicketNumber": "T-1", "Title": "Old VPN issue", "QueueName": "Service Desk", "AssigneeName": "Jane Smith", "StatusName": "New", "LastActivityUpdate": "2026-05-01T10:00:00"},
		{"TicketNumber": "T-2", "Title": "Fresh issue", "QueueName": "Service Desk", "AssigneeName": "Jane Smith", "StatusName": "New", "LastActivityUpdate": "2026-06-06T10:00:00"},
		{"TicketNumber": "T-3", "Title": "Older printer issue", "QueueName": "Projects", "AssigneeName": "Bob Lee", "StatusName": "In Progress", "LastActivityUpdate": "2026-04-01T10:00:00"},
		{"TicketNumber": "T-4", "Title": "Done", "QueueName": "Projects", "StatusName": "Completed", "CompletedDate": "2026-05-01T10:00:00", "LastActivityUpdate": "2026-01-01T10:00:00"},
	}
	tests := []struct {
		name      string
		days      int
		queue     string
		assignee  string
		limit     int
		wantCount int
		wantFirst string
	}{
		{"oldest first across queues", 7, "", "", 50, 2, "T-3"},
		{"queue filter", 7, "Service Desk", "", 50, 1, "T-1"},
		{"assignee filter", 7, "", "bob lee", 50, 1, "T-3"},
		{"limit caps output", 7, "", "", 1, 1, "T-3"},
		{"nothing stale", 90, "", "", 50, 0, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := filterStaleTickets(rows, tt.days, tt.queue, tt.assignee, tt.limit, stNow)
			if got.Count != tt.wantCount {
				t.Fatalf("Count = %d, want %d", got.Count, tt.wantCount)
			}
			if tt.wantCount > 0 && got.Items[0].TicketNumber != tt.wantFirst {
				t.Errorf("first = %s, want %s", got.Items[0].TicketNumber, tt.wantFirst)
			}
			if tt.wantCount == 0 && got.Note == "" {
				t.Errorf("expected note on empty result")
			}
		})
	}
}
