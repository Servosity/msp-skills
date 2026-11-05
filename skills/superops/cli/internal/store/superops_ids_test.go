// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package store

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

// Two SuperOps records that differ only in their vendor id are two records.
// Without a declared id_field the store keyed clients and technicians on
// name, kb and service items on name, and worklogs on a content
// fingerprint, so same-named (or same-shaped) records collapsed into one row.
func TestStoreKeysEverySuperOpsResourceOnItsVendorID(t *testing.T) {
	keys := map[string]string{
		"tickets": "ticketId", "assets": "assetId", "clients": "accountId", "sites": "id", "users": "userId",
		"contracts": "contractId", "invoices": "invoiceId", "tasks": "taskId", "alerts": "id",
		"worklogs": "worklogEntryId", "technicians": "userId", "service-items": "itemId", "kb": "itemId",
		"it-docs": "itDocumentationId",
	}
	db, err := Open(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for resource, key := range keys {
		var items []json.RawMessage
		for _, id := range []string{"6028538188152340480", "6028538188152340481"} {
			b, _ := json.Marshal(map[string]any{key: id, "name": "Same Name", "title": "Same", "timespent": 30, "billable": true})
			items = append(items, b)
		}
		if _, _, err := db.UpsertBatch(resource, items); err != nil {
			t.Fatalf("%s: upsert: %v", resource, err)
		}
		var n int
		if err := db.DB().QueryRow(`SELECT COUNT(*) FROM resources WHERE resource_type = ?`, resource).Scan(&n); err != nil {
			t.Fatalf("%s: count: %v", resource, err)
		}
		if n != 2 {
			t.Fatalf("%s: stored %d rows for 2 records with distinct %s, want 2", resource, n, key)
		}
	}
}
