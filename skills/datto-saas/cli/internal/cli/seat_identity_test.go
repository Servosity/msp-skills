// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (handfixes.json vendor-unique-store-keys).

package cli

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"datto-saas-pp-cli/internal/store"
)

// Datto KB3600321361511: a user deleted and recreated keeps the old seat as
// Archived (still billable) and gets a new seat with the same mainId. Both must
// survive a sync, in either order.
func TestStoreKeepsEverySeatDattoReturns(t *testing.T) {
	seats := []string{
		`{"parent_id":"111","mainId":"c@contoso.com","name":"C","seatType":"User","seatState":"Unprotected","billable":false,"remoteId":"r-c-new"}`,
		`{"parent_id":"111","mainId":"c@contoso.com","name":"C","seatType":"User","seatState":"Archived","billable":true,"remoteId":"r-c-old"}`,
	}
	for _, order := range [][2]int{{0, 1}, {1, 0}} {
		db, err := store.Open(filepath.Join(t.TempDir(), "data.db"))
		if err != nil {
			t.Fatal(err)
		}
		for _, i := range order {
			if err := db.UpsertSeats(json.RawMessage(seats[i])); err != nil {
				t.Fatal(err)
			}
		}
		for table, want := range map[string]int{"seats": 2} {
			var n int
			if err := db.DB().QueryRow(`SELECT count(*) FROM "` + table + `"`).Scan(&n); err != nil {
				t.Fatal(err)
			}
			if n != want {
				t.Errorf("order %v: %s holds %d rows, want %d", order, table, n, want)
			}
		}
		_ = db.Close()
	}
}
