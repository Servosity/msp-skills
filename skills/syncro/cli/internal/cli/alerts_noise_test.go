// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"testing"
	"time"

	"syncro-pp-cli/internal/store"
)

func TestNovelAlertsNoiseCommand(t *testing.T) {
	inWindow := time.Now().Add(-2 * 24 * time.Hour).Format(time.RFC3339)
	outOfWindow := time.Now().Add(-60 * 24 * time.Hour).Format(time.RFC3339)

	tests := []struct {
		name      string
		seed      func(t *testing.T, db *store.Store)
		args      []string
		wantTotal int
		topCust   string
		topCount  int
	}{
		{
			name: "empty store",
			seed: func(t *testing.T, db *store.Store) {},
		},
		{
			name: "counts alerts per customer in window; old excluded",
			seed: func(t *testing.T, db *store.Store) {
				seedResource(t, db, "customers", "C1", map[string]any{"id": "C1", "business_name": "Acme"})
				seedResource(t, db, "rmm-alerts", "A1", map[string]any{"id": "A1", "customer_id": "C1", "created_at": inWindow})
				seedResource(t, db, "rmm-alerts", "A2", map[string]any{"id": "A2", "customer_id": "C1", "created_at": inWindow})
				seedResource(t, db, "rmm-alerts", "A3", map[string]any{"id": "A3", "customer_id": "C2", "created_at": inWindow})
				// out of window -> excluded
				seedResource(t, db, "rmm-alerts", "A4", map[string]any{"id": "A4", "customer_id": "C1", "created_at": outOfWindow})
			},
			args:      []string{"--window", "30d"},
			wantTotal: 3,
			topCust:   "C1",
			topCount:  2,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			db, dbPath := newNovelTestStore(t)
			tc.seed(t, db)

			cmd := newNovelAlertsNoiseCmd(&rootFlags{asJSON: true})
			args := append([]string{"--db", dbPath}, tc.args...)
			parsed, _ := execNovel(t, cmd, args)

			if got := int(jsonFloat(t, parsed, "total_alerts")); got != tc.wantTotal {
				t.Errorf("total_alerts = %d, want %d", got, tc.wantTotal)
			}
			items := jsonItems(t, parsed)
			if tc.topCust != "" {
				if len(items) == 0 {
					t.Fatalf("expected items")
				}
				top := items[0]
				if top["customer_id"] != tc.topCust {
					t.Errorf("top customer = %v, want %s", top["customer_id"], tc.topCust)
				}
				if int(top["alert_count"].(float64)) != tc.topCount {
					t.Errorf("top count = %v, want %d", top["alert_count"], tc.topCount)
				}
			}
		})
	}
}
