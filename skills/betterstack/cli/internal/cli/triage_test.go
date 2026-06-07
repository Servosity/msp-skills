// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"testing"
	"time"
)

func TestRankTriage(t *testing.T) {
	now := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)
	names := map[string]string{"m1": "api", "m2": "web"}
	tests := []struct {
		name      string
		incidents []incidentRow
		wantLen   int
		wantFirst string
		wantState string
	}{
		{
			name:    "empty",
			wantLen: 0,
		},
		{
			name: "resolved incidents are filtered out",
			incidents: []incidentRow{
				{ID: "i1", Source: "m1", ResolvedAt: "2026-06-06T01:00:00Z"},
			},
			wantLen: 0,
		},
		{
			name: "never-acknowledged ranks before acknowledged even when younger",
			incidents: []incidentRow{
				{ID: "old-acked", Source: "m1", StartedAt: "2026-06-06T00:00:00Z", AcknowledgedAt: "2026-06-06T00:10:00Z"},
				{ID: "young-unacked", Source: "m2", StartedAt: "2026-06-06T11:00:00Z"},
			},
			wantLen:   2,
			wantFirst: "young-unacked",
			wantState: "never-acknowledged",
		},
		{
			name: "within same state oldest first",
			incidents: []incidentRow{
				{ID: "young", Source: "m1", StartedAt: "2026-06-06T11:00:00Z"},
				{ID: "old", Source: "m2", StartedAt: "2026-06-06T01:00:00Z"},
			},
			wantLen:   2,
			wantFirst: "old",
			wantState: "never-acknowledged",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			items := rankTriage(tt.incidents, names, now)
			if len(items) != tt.wantLen {
				t.Fatalf("len = %d, want %d", len(items), tt.wantLen)
			}
			if tt.wantLen == 0 {
				return
			}
			if items[0].ID != tt.wantFirst {
				t.Errorf("first = %s, want %s", items[0].ID, tt.wantFirst)
			}
			if items[0].State != tt.wantState {
				t.Errorf("state = %s, want %s", items[0].State, tt.wantState)
			}
		})
	}
}

func TestRankTriageAgeAndMonitorJoin(t *testing.T) {
	now := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)
	items := rankTriage([]incidentRow{
		{ID: "i1", Source: "m1", StartedAt: "2026-06-06T10:00:00Z"},
	}, map[string]string{"m1": "api"}, now)
	if len(items) != 1 {
		t.Fatalf("len = %d, want 1", len(items))
	}
	if items[0].AgeMinutes != 120 {
		t.Errorf("age minutes = %d, want 120", items[0].AgeMinutes)
	}
	if items[0].Monitor != "api" {
		t.Errorf("monitor = %q, want api", items[0].Monitor)
	}
}
