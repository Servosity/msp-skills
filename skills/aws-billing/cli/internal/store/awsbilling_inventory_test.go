// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package store

import (
	"path/filepath"
	"testing"
)

// inventory-scan-replaces-scope: a complete scan of one account and region
// removes that scope's rows it did not return (typed table and generic
// resources) and leaves every other account and region alone.
func TestPruneInventoryRemovesOnlyUnseenRowsOfTheScope(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for _, rec := range []string{
		`{"id":"ebs|vol-keep","resource_type":"ebs","resource_id":"vol-keep","account_id":"111122223333","region":"us-east-1","monthly_waste_usd":8}`,
		`{"id":"ebs|vol-deleted","resource_type":"ebs","resource_id":"vol-deleted","account_id":"111122223333","region":"us-east-1","monthly_waste_usd":40}`,
		`{"id":"ebs|vol-other-region","resource_type":"ebs","resource_id":"vol-other-region","account_id":"111122223333","region":"eu-west-1","monthly_waste_usd":5}`,
		`{"id":"ebs|vol-other-account","resource_type":"ebs","resource_id":"vol-other-account","account_id":"444455556666","region":"us-east-1","monthly_waste_usd":5}`,
	} {
		if err := s.UpsertInventory([]byte(rec)); err != nil {
			t.Fatal(err)
		}
	}
	n, err := s.PruneInventory("111122223333", "us-east-1", map[string]bool{"ebs|vol-keep": true})
	if err != nil || n != 1 {
		t.Fatalf("PruneInventory = %d, %v; want 1 row removed", n, err)
	}
	var left []string
	rows, err := s.DB().Query(`SELECT resource_id FROM "inventory" ORDER BY resource_id`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var id string
		_ = rows.Scan(&id)
		left = append(left, id)
	}
	_ = rows.Close()
	if len(left) != 3 || left[0] != "vol-keep" || left[1] != "vol-other-account" || left[2] != "vol-other-region" {
		t.Fatalf("inventory left = %v", left)
	}
	var generic int
	if err := s.DB().QueryRow(`SELECT COUNT(*) FROM resources WHERE resource_type = 'inventory' AND data LIKE '%vol-deleted%'`).Scan(&generic); err != nil || generic != 0 {
		t.Fatalf("the generic resources row of the deleted volume is still stored (%d, %v)", generic, err)
	}
	if _, err := s.PruneInventory("", "us-east-1", nil); err == nil {
		t.Fatal("an unknown account must never prune")
	}
}
