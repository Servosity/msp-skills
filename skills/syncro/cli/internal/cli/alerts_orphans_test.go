// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"testing"
	"time"

	"syncro-pp-cli/internal/store"
)

func TestNovelAlertsOrphansCommand(t *testing.T) {
	alertTime := time.Now().Add(-1 * 24 * time.Hour)
	alertTS := alertTime.Format(time.RFC3339)
	// ticket created within proximity of the converted alert
	nearTicketTS := alertTime.Add(1 * time.Hour).Format(time.RFC3339)

	tests := []struct {
		name        string
		seed        func(t *testing.T, db *store.Store)
		args        []string
		wantOrphans int
		wantAlert   string // alert id expected as orphan
		absentAlert string // alert id that converted, must be absent
	}{
		{
			name: "empty store",
			seed: func(t *testing.T, db *store.Store) {},
		},
		{
			name: "alert without matching ticket is orphan; converted alert excluded",
			seed: func(t *testing.T, db *store.Store) {
				seedResource(t, db, "customers", "C1", map[string]any{"id": "C1", "business_name": "Acme"})
				// orphan: asset A1, no ticket for A1
				seedResource(t, db, "rmm-alerts", "AL1", map[string]any{"id": "AL1", "customer_id": "C1", "asset_id": "A1", "created_at": alertTS})
				// converted: asset A2 has a ticket within proximity
				seedResource(t, db, "rmm-alerts", "AL2", map[string]any{"id": "AL2", "customer_id": "C1", "asset_id": "A2", "created_at": alertTS})
				seedResource(t, db, "tickets", "T1", map[string]any{"id": "T1", "customer_id": "C1", "asset_id": "A2", "created_at": nearTicketTS})
			},
			args:        []string{"--window", "7d", "--proximity", "24h"},
			wantOrphans: 1,
			wantAlert:   "AL1",
			absentAlert: "AL2",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			db, dbPath := newNovelTestStore(t)
			tc.seed(t, db)

			cmd := newNovelAlertsOrphansCmd(&rootFlags{asJSON: true})
			args := append([]string{"--db", dbPath}, tc.args...)
			parsed, _ := execNovel(t, cmd, args)

			if got := int(jsonFloat(t, parsed, "total_orphans")); got != tc.wantOrphans {
				t.Errorf("total_orphans = %d, want %d", got, tc.wantOrphans)
			}
			items := jsonItems(t, parsed)
			ids := map[string]bool{}
			for _, it := range items {
				ids[it["alert_id"].(string)] = true
			}
			if tc.wantAlert != "" && !ids[tc.wantAlert] {
				t.Errorf("expected orphan %s, got %v", tc.wantAlert, ids)
			}
			if tc.absentAlert != "" && ids[tc.absentAlert] {
				t.Errorf("converted alert %s should be excluded", tc.absentAlert)
			}
		})
	}
}
