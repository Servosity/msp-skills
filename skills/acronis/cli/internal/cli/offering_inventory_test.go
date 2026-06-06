// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"testing"
)

func TestComputeOfferingInventory(t *testing.T) {
	t.Run("rolls up by offering across tenants", func(t *testing.T) {
		db := newNovelTestStore(t)
		mustExec(t, db, `INSERT INTO offering_items(id, tenants_id, data) VALUES(?,?,?)`,
			"o1", "t1", `{"name":"backup_storage","edition":"standard"}`)
		mustExec(t, db, `INSERT INTO offering_items(id, tenants_id, data) VALUES(?,?,?)`,
			"o2", "t2", `{"name":"backup_storage","edition":"advanced"}`)
		mustExec(t, db, `INSERT INTO offering_items(id, tenants_id, data) VALUES(?,?,?)`,
			"o3", "t1", `{"name":"disaster_recovery"}`)
		// second row for same tenant+offering -> items 2, tenants still 1
		mustExec(t, db, `INSERT INTO offering_items(id, tenants_id, data) VALUES(?,?,?)`,
			"o4", "t1", `{"name":"disaster_recovery"}`)

		rows, err := computeOfferingInventory(db)
		if err != nil {
			t.Fatalf("computeOfferingInventory: %v", err)
		}
		if len(rows) != 2 {
			t.Fatalf("want 2 offerings, got %+v", rows)
		}
		// backup_storage held by 2 tenants sorts first
		if rows[0].Offering != "backup_storage" || rows[0].Tenants != 2 || rows[0].Items != 2 {
			t.Fatalf("backup_storage rollup wrong: %+v", rows[0])
		}
		if len(rows[0].Editions) != 2 {
			t.Fatalf("want 2 editions for backup_storage, got %+v", rows[0].Editions)
		}
		if rows[1].Offering != "disaster_recovery" || rows[1].Tenants != 1 || rows[1].Items != 2 {
			t.Fatalf("disaster_recovery rollup wrong: %+v", rows[1])
		}
	})

	t.Run("empty store yields empty slice", func(t *testing.T) {
		db := newNovelTestStore(t)
		rows, err := computeOfferingInventory(db)
		if err != nil {
			t.Fatalf("computeOfferingInventory: %v", err)
		}
		if len(rows) != 0 {
			t.Fatalf("want 0 rows, got %+v", rows)
		}
	})

	t.Run("rows without a name are skipped", func(t *testing.T) {
		db := newNovelTestStore(t)
		mustExec(t, db, `INSERT INTO offering_items(id, tenants_id, data) VALUES(?,?,?)`,
			"o1", "t1", `{"edition":"standard"}`)

		rows, err := computeOfferingInventory(db)
		if err != nil {
			t.Fatalf("computeOfferingInventory: %v", err)
		}
		if len(rows) != 0 {
			t.Fatalf("nameless rows must be skipped, got %+v", rows)
		}
	})
}
