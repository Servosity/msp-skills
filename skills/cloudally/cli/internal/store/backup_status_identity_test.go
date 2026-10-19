// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package store

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

// The backup-statuses feed carries no id; each row must still store, one
// per (account, task, source, entity).
func TestBackupStatusRowsStoreWithoutID(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "d.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	rows := []json.RawMessage{
		json.RawMessage(`{"userId":"u1","taskId":"t1","source":"MS365","entityName":"a@contoso.com","lastBackupDate":"2026-10-01"}`),
		json.RawMessage(`{"userId":"u1","taskId":"t1","source":"MS365","entityName":"b@contoso.com","lastBackupDate":"2026-10-01"}`),
		json.RawMessage(`{"userId":"u2","taskId":"t9","source":"MS365","entityName":"a@contoso.com","lastBackupDate":"2026-10-01"}`),
	}
	for _, res := range []string{"distributor_accounts-backup-statuses", "reseller_accounts-backup-statuses", "distributor_resellers-backup-statuses"} {
		stored, _, err := s.UpsertBatch(res, rows)
		if err != nil || stored != 3 {
			t.Fatalf("%s: stored %d, err %v; want 3", res, stored, err)
		}
		// A re-sync updates in place rather than adding rows.
		if stored, _, _ := s.UpsertBatch(res, rows[:1]); stored != 1 {
			t.Fatalf("%s: re-sync stored %d", res, stored)
		}
		n := 0
		if err := s.ListScan(res, func(string, json.RawMessage) bool { n++; return true }); err != nil || n != 3 {
			t.Fatalf("%s: %d rows after re-sync, err %v; want 3", res, n, err)
		}
	}
}
