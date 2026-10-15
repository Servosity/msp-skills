// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/sophos-central/handfixes.json
// under the "secret-responses-never-persisted" entry.

package store

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const storeSentinel = "SENTINEL-MIGRATION-TOK-store"

// TestMigrationTokenNeverStored writes a migration job through both the batch
// path sync uses and the typed single-row writer, then scans the database file
// bytes (resources, FTS and the typed table) for the token.
func TestMigrationTokenNeverStored(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "data.db")
	s, err := OpenWithContext(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	row := json.RawMessage(`{"id":"m1","status":"pending","token":"` + storeSentinel + `"}`)
	if _, _, _, err := s.UpsertBatchDetailed("migrations", []json.RawMessage{row}); err != nil {
		t.Fatalf("batch upsert: %v", err)
	}
	row2 := json.RawMessage(`{"id":"m2","status":"pending","token":"` + storeSentinel + `"}`)
	if err := s.UpsertMigrations(row2); err != nil {
		t.Fatalf("typed upsert: %v", err)
	}
	var status string
	if err := s.db.QueryRow(`SELECT status FROM migrations WHERE id = 'm1'`).Scan(&status); err != nil || status != "pending" {
		t.Fatalf("row was not stored (status=%q err=%v); the guard must drop the token, not the row", status, err)
	}
	_ = s.Close()
	matches, _ := filepath.Glob(dbPath + "*")
	for _, f := range matches {
		b, _ := os.ReadFile(f)
		if strings.Contains(string(b), storeSentinel) {
			t.Fatalf("migration token persisted in %s", f)
		}
	}
}

func TestRedactStoredSecretsKeepsOrdinaryRows(t *testing.T) {
	in := json.RawMessage(`{"id":"e1","hostname":"WS-1"}`)
	if got := redactStoredSecrets(in); string(got) != string(in) {
		t.Fatalf("ordinary row rewritten: %s", got)
	}
	got := redactStoredSecrets(json.RawMessage(`{"id":"x","nested":{"previousPasswords":[{"password":"p"}]}}`))
	if strings.Contains(string(got), "password") || strings.Contains(string(got), "previousPasswords") {
		t.Fatalf("nested secret survived: %s", got)
	}
}
