// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"testing"
	"time"

	"syncro-pp-cli/internal/store"
)

// seedInvoice inserts an invoices row with the extracted columns the billing
// commands query.
func seedInvoice(t *testing.T, db *store.Store, id string, isPaid int, total string, dueDate time.Time, ticketID int, customerID string) {
	t.Helper()
	data := map[string]any{
		"id":          id,
		"customer_id": customerID,
		"ticket_id":   ticketID,
		"total":       total,
	}
	_, err := db.DB().Exec(
		`INSERT INTO invoices(id, data, is_paid, total, due_date, ticket_id) VALUES(?, ?, ?, ?, ?, ?)`,
		id, string(mustJSON(t, data)), isPaid, total, dueDate.Format(time.RFC3339), ticketID,
	)
	if err != nil {
		t.Fatalf("seed invoice %s: %v", id, err)
	}
}

func TestNovelBillingArAgingCommand(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name         string
		seed         func(t *testing.T, db *store.Store)
		wantOutstand float64
		wantTotalCnt int
		want0to30    int
		want90plus   int
	}{
		{
			name: "empty store returns zero buckets",
			seed: func(t *testing.T, db *store.Store) {},
		},
		{
			name: "buckets and totals computed; paid excluded",
			seed: func(t *testing.T, db *store.Store) {
				seedInvoice(t, db, "1", 0, "100.00", now.AddDate(0, 0, -5), 0, "10")   // 0-30
				seedInvoice(t, db, "2", 0, "200.00", now.AddDate(0, 0, -45), 0, "10")  // 31-60
				seedInvoice(t, db, "3", 0, "300.00", now.AddDate(0, 0, -120), 0, "10") // 90+
				seedInvoice(t, db, "4", 1, "999.00", now.AddDate(0, 0, -200), 0, "10") // PAID - excluded
			},
			wantOutstand: 600.0,
			wantTotalCnt: 3,
			want0to30:    1,
			want90plus:   1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			db, dbPath := newNovelTestStore(t)
			tc.seed(t, db)

			cmd := newNovelBillingArAgingCmd(&rootFlags{asJSON: true})
			parsed, _ := execNovel(t, cmd, []string{"--db", dbPath})

			if got := jsonFloat(t, parsed, "total_outstanding"); got != tc.wantOutstand {
				t.Errorf("total_outstanding = %v, want %v", got, tc.wantOutstand)
			}
			if got := int(jsonFloat(t, parsed, "total_count")); got != tc.wantTotalCnt {
				t.Errorf("total_count = %d, want %d", got, tc.wantTotalCnt)
			}
			buckets, _ := parsed["buckets"].([]any)
			if len(buckets) != 4 {
				t.Fatalf("expected 4 buckets, got %d", len(buckets))
			}
			b0 := buckets[0].(map[string]any)
			if int(b0["count"].(float64)) != tc.want0to30 {
				t.Errorf("0-30 count = %v, want %d", b0["count"], tc.want0to30)
			}
			b90 := buckets[3].(map[string]any)
			if int(b90["count"].(float64)) != tc.want90plus {
				t.Errorf("90+ count = %v, want %d", b90["count"], tc.want90plus)
			}
		})
	}
}
