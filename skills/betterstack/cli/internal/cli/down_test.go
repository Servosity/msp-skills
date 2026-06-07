// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import "testing"

func TestBuildDownReport(t *testing.T) {
	tests := []struct {
		name        string
		monitors    []monitorRow
		incidents   []incidentRow
		onCalls     []onCallRow
		wantItems   int
		wantUnpaged int
		wantUnacked int
		wantCovered bool
	}{
		{
			name:      "empty store",
			wantItems: 0,
		},
		{
			name: "healthy fleet stays off the board",
			monitors: []monitorRow{
				{ID: "1", Name: "api", Status: "up", PolicyID: "p1"},
				{ID: "2", Name: "web", Status: "up", Email: true},
			},
			onCalls:     []onCallRow{{ID: "c1", OnCallUsers: 1}},
			wantItems:   0,
			wantCovered: true,
		},
		{
			name: "down monitor with no policy and no channel pages nobody",
			monitors: []monitorRow{
				{ID: "1", Name: "api", Status: "down"},
			},
			incidents: []incidentRow{
				{ID: "i1", Source: "1", StartedAt: "2026-06-06T00:00:00Z"},
			},
			wantItems:   1,
			wantUnpaged: 1,
			wantUnacked: 1,
		},
		{
			name: "up monitor with open incident is degraded",
			monitors: []monitorRow{
				{ID: "1", Name: "api", Status: "up", PolicyID: "p1"},
			},
			incidents: []incidentRow{
				{ID: "i1", Source: "1", StartedAt: "2026-06-06T00:00:00Z", AcknowledgedAt: "2026-06-06T00:05:00Z"},
			},
			wantItems: 1,
		},
		{
			name: "paused monitors are excluded",
			monitors: []monitorRow{
				{ID: "1", Name: "api", Status: "down", Paused: true},
			},
			wantItems: 0,
		},
		{
			name: "resolved incidents do not count",
			monitors: []monitorRow{
				{ID: "1", Name: "api", Status: "up", PolicyID: "p1"},
			},
			incidents: []incidentRow{
				{ID: "i1", Source: "1", ResolvedAt: "2026-06-06T01:00:00Z"},
			},
			wantItems: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rep := buildDownReport(tt.monitors, tt.incidents, tt.onCalls)
			if len(rep.Items) != tt.wantItems {
				t.Fatalf("items = %d, want %d", len(rep.Items), tt.wantItems)
			}
			if rep.UnpagedCount != tt.wantUnpaged {
				t.Errorf("unpaged = %d, want %d", rep.UnpagedCount, tt.wantUnpaged)
			}
			if rep.UnackedCount != tt.wantUnacked {
				t.Errorf("unacked = %d, want %d", rep.UnackedCount, tt.wantUnacked)
			}
			if rep.OnCallCovered != tt.wantCovered {
				t.Errorf("covered = %v, want %v", rep.OnCallCovered, tt.wantCovered)
			}
		})
	}
}

func TestBuildDownReportOrdering(t *testing.T) {
	monitors := []monitorRow{
		{ID: "1", Name: "zz-acked", Status: "down", PolicyID: "p1"},
		{ID: "2", Name: "aa-unpaged", Status: "down"},
		{ID: "3", Name: "mm-unacked", Status: "down", Email: true},
	}
	incidents := []incidentRow{
		{ID: "i1", Source: "1", AcknowledgedAt: "2026-06-06T00:05:00Z"},
	}
	rep := buildDownReport(monitors, incidents, nil)
	if len(rep.Items) != 3 {
		t.Fatalf("items = %d, want 3", len(rep.Items))
	}
	if rep.Items[0].MonitorID != "2" {
		t.Errorf("first item should be the pages-nobody monitor, got %s", rep.Items[0].MonitorID)
	}
}
