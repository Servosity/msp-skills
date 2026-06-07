// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import "testing"

func TestBuildGroupHealth(t *testing.T) {
	tests := []struct {
		name            string
		monitorGroups   []groupRow
		heartbeatGroups []groupRow
		monitors        []monitorRow
		heartbeats      []heartbeatRow
		hbGroupIDs      map[string]string
		incidents       []incidentRow
		wantRows        int
		wantFirst       string
	}{
		{
			name:     "empty store",
			wantRows: 0,
		},
		{
			name:          "group with open incidents sorts first",
			monitorGroups: []groupRow{{ID: "g1", Name: "client-a"}, {ID: "g2", Name: "client-b"}},
			monitors: []monitorRow{
				{ID: "1", GroupID: "g1", Status: "up"},
				{ID: "2", GroupID: "g2", Status: "down"},
			},
			incidents: []incidentRow{
				{ID: "i1", Source: "2", StartedAt: "2026-06-06T00:00:00Z"},
			},
			wantRows:  2,
			wantFirst: "client-b",
		},
		{
			name: "ungrouped members aggregate under synthetic row",
			monitors: []monitorRow{
				{ID: "1", Status: "up"},
				{ID: "2", Status: "down"},
			},
			wantRows:  1,
			wantFirst: "(ungrouped)",
		},
		{
			name:            "heartbeat groups roll up independently",
			heartbeatGroups: []groupRow{{ID: "h1", Name: "crons"}},
			heartbeats:      []heartbeatRow{{ID: "hb1", Status: "up"}, {ID: "hb2", Status: "missing"}},
			hbGroupIDs:      map[string]string{"hb1": "h1", "hb2": "h1"},
			wantRows:        1,
			wantFirst:       "crons",
		},
		{
			name:            "case-variant Up status is not down",
			heartbeatGroups: []groupRow{{ID: "h1", Name: "crons"}},
			heartbeats:      []heartbeatRow{{ID: "hb1", Status: "Up"}},
			hbGroupIDs:      map[string]string{"hb1": "h1"},
			wantRows:        1,
			wantFirst:       "crons",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rows := buildGroupHealth(tt.monitorGroups, tt.heartbeatGroups, tt.monitors, tt.heartbeats, tt.hbGroupIDs, tt.incidents)
			if len(rows) != tt.wantRows {
				t.Fatalf("rows = %d, want %d", len(rows), tt.wantRows)
			}
			if tt.wantRows > 0 && rows[0].Name != tt.wantFirst {
				t.Errorf("first = %s, want %s", rows[0].Name, tt.wantFirst)
			}
		})
	}
}

func TestBuildGroupHealthCounts(t *testing.T) {
	rows := buildGroupHealth(
		[]groupRow{{ID: "g1", Name: "client-a"}},
		nil,
		[]monitorRow{
			{ID: "1", GroupID: "g1", Status: "up"},
			{ID: "2", GroupID: "g1", Status: "down"},
			{ID: "3", GroupID: "g1", Status: "up", Paused: true},
		},
		nil, nil,
		[]incidentRow{
			{ID: "i1", Source: "2", StartedAt: "2026-06-06T00:00:00Z"},
			{ID: "i2", Source: "2", ResolvedAt: "2026-06-06T01:00:00Z"},
		},
	)
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	r := rows[0]
	if r.Total != 3 || r.Up != 1 || r.Down != 1 || r.Paused != 1 {
		t.Errorf("counts = total %d up %d down %d paused %d, want 3/1/1/1", r.Total, r.Up, r.Down, r.Paused)
	}
	if r.OpenIncidents != 1 {
		t.Errorf("open incidents = %d, want 1 (resolved must not count)", r.OpenIncidents)
	}
}
