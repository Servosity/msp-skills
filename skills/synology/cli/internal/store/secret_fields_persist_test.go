// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/synology/handfixes.json
// under the "secret-fields-never-persisted" entry.

package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestStoreNeverPersistsSecretFields: sync and every live read write DSM rows
// through to data.db, which MCP sql serves even in read-only mode; a
// credential-named field must never land there (or in the FTS index).
func TestStoreNeverPersistsSecretFields(t *testing.T) {
	const sentinel = "SENTINEL-SECRET-7731"
	dir := t.TempDir()
	path := filepath.Join(dir, "data.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Upsert("user", "alice", json.RawMessage(`{"name":"alice","passwd":"`+sentinel+`"}`)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.UpsertBatch("folder", []json.RawMessage{json.RawMessage(`{"id":"share1","name":"ActiveBackupforBusiness","enc_passwd":"` + sentinel + `"}`)}); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := s.DB().QueryRow(`SELECT COUNT(*) FROM resources WHERE data LIKE ?`, "%"+sentinel+"%").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("%d stored row(s) carry the secret", n)
	}
	if err := s.DB().QueryRow(`SELECT COUNT(*) FROM resources WHERE data LIKE '%alice%'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("the row itself was not stored (n=%d, err=%v)", n, err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		b, _ := os.ReadFile(filepath.Join(dir, e.Name()))
		if strings.Contains(string(b), sentinel) {
			t.Fatalf("%s on disk carries the secret", e.Name())
		}
	}
}
