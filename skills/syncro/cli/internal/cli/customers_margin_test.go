// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"testing"
	"time"

	"syncro-pp-cli/internal/store"
)

func TestNovelCustomersMarginCommand(t *testing.T) {
	tests := []struct {
		name      string
		seed      func(t *testing.T, db *store.Store)
		wantItems int
		// expectations for the top (highest revenue) customer
		wantTopRevenue float64
		wantTopHours   float64
		wantTopRPHNull bool
	}{
		{
			name: "empty store",
			seed: func(t *testing.T, db *store.Store) {},
		},
		{
			name: "revenue and hours per customer; rev/hr null when no hours",
			seed: func(t *testing.T, db *store.Store) {
				seedResource(t, db, "customers", "C1", map[string]any{"id": "C1", "business_name": "Acme"})
				seedResource(t, db, "customers", "C2", map[string]any{"id": "C2", "business_name": "Beta"})
				seedResource(t, db, "tickets", "T1", map[string]any{"id": "T1", "customer_id": "C1"})
				// C1: $1000 revenue, 10h labor -> rev/hr 100
				seedInvoice(t, db, "INV1", 0, "1000.00", time.Now(), 0, "C1")
				seedTimerEntry(t, db, "E1", "T1", map[string]any{"duration": 36000, "start_at": time.Now().Format(time.RFC3339)})
				// C2: $100 revenue, no labor -> rev/hr null
				seedInvoice(t, db, "INV2", 0, "100.00", time.Now(), 0, "C2")
			},
			wantItems:      2,
			wantTopRevenue: 1000.0,
			wantTopHours:   10.0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			db, dbPath := newNovelTestStore(t)
			tc.seed(t, db)

			cmd := newNovelCustomersMarginCmd(&rootFlags{asJSON: true})
			parsed, _ := execNovel(t, cmd, []string{"--db", dbPath})

			items := jsonItems(t, parsed)
			if len(items) != tc.wantItems {
				t.Fatalf("items = %d, want %d (%v)", len(items), tc.wantItems, items)
			}
			if tc.wantItems == 0 {
				return
			}
			top := items[0] // sorted by revenue desc
			if rev := top["revenue"].(float64); rev != tc.wantTopRevenue {
				t.Errorf("top revenue = %v, want %v", rev, tc.wantTopRevenue)
			}
			if h := top["labor_hours"].(float64); h != tc.wantTopHours {
				t.Errorf("top labor_hours = %v, want %v", h, tc.wantTopHours)
			}
			if rph, ok := top["revenue_per_hour"].(float64); !ok || rph != 100.0 {
				t.Errorf("top revenue_per_hour = %v, want 100", top["revenue_per_hour"])
			}
			// the no-labor customer must have null revenue_per_hour
			last := items[len(items)-1]
			if last["revenue_per_hour"] != nil {
				t.Errorf("no-labor customer revenue_per_hour = %v, want null", last["revenue_per_hour"])
			}
		})
	}
}
