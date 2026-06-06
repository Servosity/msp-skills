// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"testing"

	"syncro-pp-cli/internal/store"
)

func TestNovelBillingUninvoicedCommand(t *testing.T) {
	tests := []struct {
		name           string
		seed           func(t *testing.T, db *store.Store)
		wantItems      int
		wantTotalHours float64
		checkCustomer  string // customer id to assert hours for
		wantCustHours  float64
	}{
		{
			name: "empty store returns no items",
			seed: func(t *testing.T, db *store.Store) {},
		},
		{
			name: "groups uninvoiced hours by customer; invoiced excluded",
			seed: func(t *testing.T, db *store.Store) {
				seedResource(t, db, "customers", "C1", map[string]any{"id": "C1", "business_name": "Acme"})
				seedResource(t, db, "tickets", "T1", map[string]any{"id": "T1", "customer_id": "C1"})
				seedResource(t, db, "tickets", "T2", map[string]any{"id": "T2", "customer_id": "C1"})
				// 1h uninvoiced
				seedTimerEntry(t, db, "E1", "T1", map[string]any{"duration": 3600})
				// 2h uninvoiced
				seedTimerEntry(t, db, "E2", "T2", map[string]any{"duration": 7200})
				// already invoiced -> excluded
				seedTimerEntry(t, db, "E3", "T1", map[string]any{"duration": 9999, "invoice_id": 55})
			},
			wantItems:      1,
			wantTotalHours: 3.0,
			checkCustomer:  "C1",
			wantCustHours:  3.0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			db, dbPath := newNovelTestStore(t)
			tc.seed(t, db)

			cmd := newNovelBillingUninvoicedCmd(&rootFlags{asJSON: true})
			parsed, _ := execNovel(t, cmd, []string{"--db", dbPath})

			items := jsonItems(t, parsed)
			if len(items) != tc.wantItems {
				t.Fatalf("items = %d, want %d (%v)", len(items), tc.wantItems, items)
			}
			if got := jsonFloat(t, parsed, "total_uninvoiced_hours"); got != tc.wantTotalHours {
				t.Errorf("total_uninvoiced_hours = %v, want %v", got, tc.wantTotalHours)
			}
			if tc.checkCustomer != "" {
				found := false
				for _, it := range items {
					if it["customer_id"] == tc.checkCustomer {
						found = true
						if h := it["uninvoiced_hours"].(float64); h != tc.wantCustHours {
							t.Errorf("customer %s hours = %v, want %v", tc.checkCustomer, h, tc.wantCustHours)
						}
					}
				}
				if !found {
					t.Errorf("customer %s not in items", tc.checkCustomer)
				}
			}
		})
	}
}
