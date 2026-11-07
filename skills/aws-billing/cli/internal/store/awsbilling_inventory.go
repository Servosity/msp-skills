// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package store

// aws-billing handfix (inventory-scan-replaces-scope): an inventory scan is a
// snapshot of one account and region. Rows were only ever upserted, so a
// volume, snapshot or address deleted in AWS stayed in the store as waste and
// every later `waste` command listed and totalled it. After a COMPLETE scan
// (no failure at all), PruneInventory removes the rows of that account and
// region the scan did not return; an incomplete scan prunes nothing. S3
// buckets are listed account-wide and stored under their own region (Codex
// r1 class wrong-resource-region), so a complete scan prunes the account's
// buckets in every region.

import (
	"database/sql"
	"fmt"
)

// PruneInventory deletes the inventory rows of account and region whose
// resource_type|resource_id is not in seen, from the typed table and the
// generic resources table, and returns how many it removed.
func (s *Store) PruneInventory(account, region string, seen map[string]bool) (int, error) {
	if account == "" {
		return 0, fmt.Errorf("prune inventory: no account")
	}
	s.lockForWrite()
	defer s.unlockAfterWrite()
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	n, err := pruneInventoryTx(tx, account, region, seen)
	if err != nil {
		return 0, err
	}
	return n, tx.Commit()
}

// pruneInventoryTx is PruneInventory inside a caller's transaction.
func pruneInventoryTx(tx *sql.Tx, account, region string, seen map[string]bool) (int, error) {
	if account == "" {
		return 0, fmt.Errorf("prune inventory: no account")
	}
	rows, err := tx.Query(`SELECT "id", COALESCE("resource_type", ''), COALESCE("resource_id", '') FROM "inventory" WHERE "account_id" = ? AND ("region" = ? OR "resource_type" = 's3')`, account, region)
	if err != nil {
		return 0, err
	}
	var stale []string
	for rows.Next() {
		var id, typ, rid string
		if err := rows.Scan(&id, &typ, &rid); err != nil {
			_ = rows.Close()
			return 0, err
		}
		if !seen[id] && !seen[typ+"|"+rid] { // the stored ID (a withheld:<n> one too) or the resource
			stale = append(stale, id)
		}
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}
	if err := deleteResourceRows(tx, "inventory", stale); err != nil {
		return 0, err
	}
	return len(stale), nil
}
