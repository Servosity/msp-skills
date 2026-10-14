// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package store

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

// hand-fix guard (handfixes.json secret-fields-never-persisted): a sentinel
// credential in a synced policy or notification row must not land anywhere
// in the SQLite file - not the generic blob, the typed columns or the FTS index.
func TestSecretFieldsNeverPersisted(t *testing.T) {
	const sentinel = "SENTINEL-TAMPER-PW-9f3"
	dbPath := filepath.Join(t.TempDir(), "data.db")
	s, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()

	policy := json.RawMessage(`{"id":"p1","policy_name":"Default","as_uninstall_password":"` + sentinel + `","secret":"` + sentinel + `","secret_hash":"` + sentinel + `","policy_secret_set":true,"big":12345678901234567890}`)
	if err := s.UpsertPolicies(policy); err != nil {
		t.Fatalf("UpsertPolicies: %v", err)
	}
	sub := json.RawMessage(`{"id":"n1","name":"hook","nested":{"secret_token":"` + sentinel + `"},"custom_headers":[{"key":"authorization","value":"Bearer ` + sentinel + `"}]}`)
	if _, _, err := s.UpsertBatch("notifications", []json.RawMessage{sub}); err != nil {
		t.Fatalf("UpsertBatch: %v", err)
	}
	if err := s.Upsert("sites", "s1", json.RawMessage(`{"id":"s1","secret":"`+sentinel+`"}`)); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	got, err := s.Get("policies", "p1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !strings.Contains(string(got), `"policy_name":"Default"`) || !strings.Contains(string(got), "12345678901234567890") {
		t.Fatalf("non-secret fields lost: %s", got)
	}

	rows, err := s.DB().Query(`SELECT name FROM sqlite_master WHERE type='table'`)
	if err != nil {
		t.Fatal(err)
	}
	var tables []string
	for rows.Next() {
		var n string
		_ = rows.Scan(&n)
		tables = append(tables, n)
	}
	rows.Close()
	for _, tbl := range tables {
		r, err := s.DB().Query(`SELECT * FROM "` + tbl + `"`)
		if err != nil {
			continue
		}
		cols, _ := r.Columns()
		for r.Next() {
			vals := make([]any, len(cols))
			ptrs := make([]any, len(cols))
			for i := range vals {
				ptrs[i] = &vals[i]
			}
			_ = r.Scan(ptrs...)
			for i, v := range vals {
				var str string
				switch tv := v.(type) {
				case string:
					str = tv
				case []byte:
					str = string(tv)
				}
				if strings.Contains(str, sentinel) {
					t.Errorf("secret persisted in %s.%s", tbl, cols[i])
				}
			}
		}
		r.Close()
	}
}
