// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored tests for the Kaseya BMS novel commands.

package cli

import (
	"testing"
	"time"
)

var qhNow = time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC)

func qhTicket(queue, priority, status, activity, completed string) map[string]any {
	m := map[string]any{
		"QueueName": queue, "PriorityName": priority, "StatusName": status,
		"OpenDate": "2026-05-01T09:00:00",
	}
	if activity != "" {
		m["LastActivityUpdate"] = activity
	}
	if completed != "" {
		m["CompletedDate"] = completed
	}
	return m
}

func TestAggregateQueueHealth(t *testing.T) {
	tests := []struct {
		name       string
		rows       []map[string]any
		staleDays  int
		queue      string
		wantOpen   int
		wantStale  int
		wantQueues int
	}{
		{
			name: "open tickets grouped and stale flagged",
			rows: []map[string]any{
				qhTicket("Service Desk", "High", "New", "2026-06-06T10:00:00", ""),
				qhTicket("Service Desk", "Low", "In Progress", "2026-05-01T10:00:00", ""),
				qhTicket("Projects", "Medium", "New", "2026-06-05T10:00:00", ""),
				qhTicket("Service Desk", "High", "Completed", "2026-06-01T10:00:00", "2026-06-01T10:00:00"),
			},
			staleDays: 7, wantOpen: 3, wantStale: 1, wantQueues: 2,
		},
		{
			name: "queue filter narrows",
			rows: []map[string]any{
				qhTicket("Service Desk", "High", "New", "2026-06-06T10:00:00", ""),
				qhTicket("Projects", "Medium", "New", "2026-06-05T10:00:00", ""),
			},
			staleDays: 7, queue: "projects", wantOpen: 1, wantStale: 0, wantQueues: 1,
		},
		{
			name:      "empty mirror produces note",
			rows:      nil,
			staleDays: 7, wantOpen: 0, wantStale: 0, wantQueues: 0,
		},
		{
			name: "camelCase wire keys are read",
			rows: []map[string]any{
				{"queueName": "Helpdesk", "priorityName": "High", "statusName": "New", "lastActivityUpdate": "2026-06-06T10:00:00"},
			},
			staleDays: 7, wantOpen: 1, wantStale: 0, wantQueues: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := aggregateQueueHealth(tt.rows, tt.staleDays, tt.queue, qhNow)
			if got.TotalOpen != tt.wantOpen {
				t.Errorf("TotalOpen = %d, want %d", got.TotalOpen, tt.wantOpen)
			}
			if got.StaleTotal != tt.wantStale {
				t.Errorf("StaleTotal = %d, want %d", got.StaleTotal, tt.wantStale)
			}
			if len(got.Queues) != tt.wantQueues {
				t.Errorf("len(Queues) = %d, want %d", len(got.Queues), tt.wantQueues)
			}
			if tt.wantOpen == 0 && got.Note == "" {
				t.Errorf("expected note on empty result")
			}
		})
	}
}
