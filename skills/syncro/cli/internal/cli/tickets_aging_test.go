// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"testing"
	"time"

	"syncro-pp-cli/internal/store"
)

func TestNovelTicketsAgingCommand(t *testing.T) {
	oldTS := time.Now().Add(-72 * time.Hour).Format(time.RFC3339)
	recentTS := time.Now().Add(-1 * time.Hour).Format(time.RFC3339)

	tests := []struct {
		name        string
		seed        func(t *testing.T, db *store.Store)
		args        []string
		wantTotal   int
		wantTicket  string
		absentTickt string
	}{
		{
			name: "empty store",
			seed: func(t *testing.T, db *store.Store) {},
		},
		{
			name: "open stale ticket surfaces; recently-commented and closed excluded",
			seed: func(t *testing.T, db *store.Store) {
				seedResource(t, db, "customers", "C1", map[string]any{"id": "C1", "business_name": "Acme"})
				// open, last activity old via updated_at -> aging
				seedResource(t, db, "tickets", "1", map[string]any{"id": "1", "status": "New", "updated_at": oldTS, "customer_id": "C1", "number": "T-1"})
				// open, old ticket BUT recent comment -> excluded
				seedResource(t, db, "tickets", "2", map[string]any{"id": "2", "status": "In Progress", "updated_at": oldTS, "customer_id": "C1", "number": "T-2"})
				seedComment(t, db, "CM1", "2", map[string]any{"created_at": recentTS, "body": "ping"})
				// closed -> excluded
				seedResource(t, db, "tickets", "3", map[string]any{"id": "3", "status": "Closed", "updated_at": oldTS, "customer_id": "C1", "number": "T-3"})
			},
			args:        []string{"--no-comment", "48h"},
			wantTotal:   1,
			wantTicket:  "1",
			absentTickt: "2",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			db, dbPath := newNovelTestStore(t)
			tc.seed(t, db)

			cmd := newNovelTicketsAgingCmd(&rootFlags{asJSON: true})
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
				t.Errorf("expected ticket %s, got %v", tc.wantTicket, ids)
			}
			if tc.absentTickt != "" && ids[tc.absentTickt] {
				t.Errorf("recently-commented ticket %s should be excluded", tc.absentTickt)
			}
		})
	}
}
