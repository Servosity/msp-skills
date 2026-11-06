// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package store

// r1b-inventory-region-resolved: a stored inventory read names its region and
// answers from that region's scan alone (S3 buckets, listed account-wide,
// included), never from every scanned region.

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestR1bInventoryReadAnswersOnlyItsRegion(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	const acct = "111122223333"
	east := []json.RawMessage{
		json.RawMessage(`{"id":"ebs|vol-east","resource_type":"ebs","resource_id":"vol-east","account_id":"111122223333","region":"us-east-1","monthly_waste_usd":8}`),
		json.RawMessage(`{"id":"s3|bucket-a","resource_type":"s3","resource_id":"bucket-a","account_id":"111122223333","region":"eu-west-1"}`),
	}
	if _, err := s.ReplaceInventoryScan(acct, "us-east-1", east, map[string]bool{"ebs|vol-east": true, "s3|bucket-a": true}, true, ""); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, _, _, err := s.InventoryIfScanned(ctx, acct, "", "", "", false); err == nil {
		t.Error("a read naming no region answered (it would answer for every scanned region)")
	}
	if rows, ok, _, err := s.InventoryIfScanned(ctx, acct, "us-west-2", "", "", false); err != nil || ok || len(rows) != 0 {
		t.Errorf("a us-west-2 read with only a us-east-1 scan stored: ok=%v rows=%v err=%v", ok, rows, err)
	}
	rows, ok, _, err := s.InventoryIfScanned(ctx, acct, "us-east-1", "", "", false)
	if err != nil || !ok || len(rows) != 2 {
		t.Fatalf("the us-east-1 read: ok=%v rows=%v err=%v", ok, rows, err)
	}
	west := []json.RawMessage{json.RawMessage(`{"id":"ebs|vol-west","resource_type":"ebs","resource_id":"vol-west","account_id":"111122223333","region":"us-west-2","monthly_waste_usd":3}`)}
	if _, err := s.ReplaceInventoryScan(acct, "us-west-2", west, map[string]bool{"ebs|vol-west": true}, true, ""); err != nil {
		t.Fatal(err)
	}
	rows, ok, _, err = s.InventoryIfScanned(ctx, acct, "us-west-2", "ebs", "", false)
	if err != nil || !ok || len(rows) != 1 || rows[0].ResourceID != "vol-west" {
		t.Errorf("with both regions stored, the us-west-2 read answered %v (ok=%v err=%v), want vol-west alone", rows, ok, err)
	}
}
