// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"testing"
	"time"

	"syncro-pp-cli/internal/store"
)

func TestNovelBillingDriftCommand(t *testing.T) {
	old := time.Now().AddDate(0, 0, -30).Format(time.RFC3339)
	recent := time.Now().Format(time.RFC3339)

	tests := []struct {
		name        string
		seed        func(t *testing.T, db *store.Store)
		args        []string
		wantTotal   int
		wantTicket  string // a ticket id that must appear
		absentTickt string // a ticket id that must NOT appear
	}{
		{
			name: "empty store",
			seed: func(t *testing.T, db *store.Store) {},
			args: []string{},
		},
		{
			name: "closed old uninvoiced ticket surfaces; invoiced excluded; recent excluded",
			seed: func(t *testing.T, db *store.Store) {
				// closed long ago, no invoice -> drift
				seedResource(t, db, "tickets", "100", map[string]any{"id": "100", "status": "Closed", "resolved_at": old, "customer_id": "C1", "number": "T-100"})
				// resolved long ago BUT has an invoice -> excluded
				seedResource(t, db, "tickets", "200", map[string]any{"id": "200", "status": "Resolved", "resolved_at": old, "customer_id": "C1", "number": "T-200"})
				seedInvoice(t, db, "INV1", 0, "50.00", time.Now(), 200, "C1")
				// closed recently -> excluded by cutoff
				seedResource(t, db, "tickets", "300", map[string]any{"id": "300", "status": "Closed", "resolved_at": recent, "customer_id": "C1", "number": "T-300"})
				// open ticket -> excluded by status
				seedResource(t, db, "tickets", "400", map[string]any{"id": "400", "status": "New", "updated_at": old, "customer_id": "C1", "number": "T-400"})
				seedResource(t, db, "customers", "C1", map[string]any{"id": "C1", "business_name": "Acme"})
			},
			args:        []string{"--closed-before", "14d"},
			wantTotal:   1,
			wantTicket:  "100",
			absentTickt: "200",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			db, dbPath := newNovelTestStore(t)
			tc.seed(t, db)

			cmd := newNovelBillingDriftCmd(&rootFlags{asJSON: true})
			args := append([]string{"--db", dbPath}, tc.args...)
			parsed, _ := execNovel(t, cmd, args)

			if got := int(jsonFloat(t, parsed, "total_count")); got != tc.wantTotal {
				t.Errorf("total_count = %d, want %d", got, tc.wantTotal)
			}
			items := jsonItems(t, parsed)
			ids := map[string]bool{}
			for _, it := range items {
				ids[it["ticket_id"].(string)] = true
			}
			if tc.wantTicket != "" && !ids[tc.wantTicket] {
				t.Errorf("expected ticket %s in drift, got %v", tc.wantTicket, ids)
			}
			if tc.absentTickt != "" && ids[tc.absentTickt] {
				t.Errorf("invoiced ticket %s should be excluded", tc.absentTickt)
			}
		})
	}
}
