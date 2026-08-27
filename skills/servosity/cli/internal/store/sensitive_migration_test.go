// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package store

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

func TestSchemaV10PurgesMirroredAPITokens(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "data.db")
	seed, err := Open(dbPath)
	if err != nil {
		t.Fatalf("create current store: %v", err)
	}
	if err := seed.Close(); err != nil {
		t.Fatalf("close current store: %v", err)
	}

	raw, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open raw store: %v", err)
	}
	stmts := []string{
		`INSERT INTO resources (id, resource_type, data) VALUES ('safe', 'issues', '{"id":"safe","name":"preserve me"}')`,
		`INSERT INTO resources (id, resource_type, data) VALUES ('token-direct', 'current-user-api-token', '{"token":"fixturesecret-direct"}')`,
		`INSERT INTO resources (id, resource_type, data) VALUES ('token-user', 'current-user', '{"id":"token-user","token":"fixturesecret-user"}')`,
		`INSERT INTO resources (id, resource_type, data) VALUES ('token-nested', 'current-user', '{"id":"token-nested","data":{"token":"fixturesecret-nested"}}')`,
		`INSERT INTO resources (id, resource_type, data) VALUES ('normal-user', 'current-user', '{"id":"normal-user","first_name":"preserve"}')`,
		`INSERT INTO current_user (id, data) VALUES ('token-user', '{"id":"token-user","token":"fixturesecret-user"}')`,
		`INSERT INTO current_user (id, data) VALUES ('normal-user', '{"id":"normal-user","first_name":"preserve"}')`,
		`INSERT INTO sync_state (resource_type, last_cursor, total_count) VALUES ('current-user-api-token', 'secret-cursor', 1)`,
		`PRAGMA user_version = 9`,
	}
	for _, stmt := range stmts {
		if _, err := raw.Exec(stmt); err != nil {
			raw.Close()
			t.Fatalf("seed v9 store (%s): %v", stmt, err)
		}
	}
	for _, row := range []struct {
		id, resourceType, content string
	}{
		{"safe", "issues", "preserve me"},
		{"token-direct", "current-user-api-token", "fixturesecret direct"},
		{"token-user", "current-user", "fixturesecret user"},
		{"orphan", "current-user-api-token", "fixturesecret orphan"},
	} {
		if _, err := raw.Exec(`INSERT INTO resources_fts (rowid, id, resource_type, content) VALUES (?, ?, ?, ?)`, ftsRowID(row.resourceType, row.id), row.id, row.resourceType, row.content); err != nil {
			raw.Close()
			t.Fatalf("seed FTS row %s/%s: %v", row.resourceType, row.id, err)
		}
	}
	if err := raw.Close(); err != nil {
		t.Fatalf("close raw store: %v", err)
	}

	s, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open migrated store: %v", err)
	}
	assertSensitiveStorePurged(t, s)
	if err := s.Close(); err != nil {
		t.Fatalf("close migrated store: %v", err)
	}

	reopened, err := Open(dbPath)
	if err != nil {
		t.Fatalf("reopen migrated store: %v", err)
	}
	defer reopened.Close()
	assertSensitiveStorePurged(t, reopened)
}

func assertSensitiveStorePurged(t *testing.T, s *Store) {
	t.Helper()
	version, err := s.SchemaVersion()
	if err != nil {
		t.Fatalf("read schema version: %v", err)
	}
	if version != StoreSchemaVersion {
		t.Fatalf("schema version = %d, want %d", version, StoreSchemaVersion)
	}
	checks := []struct {
		query string
		want  int
	}{
		{`SELECT COUNT(*) FROM resources WHERE resource_type = 'current-user-api-token'`, 0},
		{`SELECT COUNT(*) FROM resources WHERE resource_type = 'current-user' AND CASE WHEN json_valid(data) THEN json_type(data, '$.token') IS NOT NULL OR json_type(data, '$.data.token') IS NOT NULL ELSE 0 END`, 0},
		{`SELECT COUNT(*) FROM current_user WHERE CASE WHEN json_valid(data) THEN json_type(data, '$.token') IS NOT NULL OR json_type(data, '$.data.token') IS NOT NULL ELSE 0 END`, 0},
		{`SELECT COUNT(*) FROM sync_state WHERE resource_type = 'current-user-api-token'`, 0},
		{`SELECT COUNT(*) FROM resources_fts WHERE resource_type = 'current-user-api-token'`, 0},
		{`SELECT COUNT(*) FROM resources_fts WHERE resources_fts MATCH 'fixturesecret'`, 0},
		{`SELECT COUNT(*) FROM resources WHERE id = 'safe' AND resource_type = 'issues'`, 1},
		{`SELECT COUNT(*) FROM resources WHERE id = 'normal-user' AND resource_type = 'current-user'`, 1},
		{`SELECT COUNT(*) FROM current_user WHERE id = 'normal-user'`, 1},
	}
	for _, check := range checks {
		var got int
		if err := s.DB().QueryRow(check.query).Scan(&got); err != nil {
			t.Fatalf("query %q: %v", check.query, err)
		}
		if got != check.want {
			t.Fatalf("query %q = %d, want %d", check.query, got, check.want)
		}
	}
}

func TestStoreRejectsSensitiveMirroredResources(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer s.Close()

	for name, write := range map[string]func() error{
		"dedicated resource": func() error {
			return s.Upsert("current-user-api-token", "token", []byte(`{"token":"fixture"}`))
		},
		"top-level token": func() error {
			return s.Upsert("current-user", "user", []byte(`{"id":"user","token":"fixture"}`))
		},
		"nested token": func() error {
			return s.Upsert("current-user", "user", []byte(`{"id":"user","data":{"token":"fixture"}}`))
		},
	} {
		if err := write(); err == nil || !strings.Contains(err.Error(), "refusing to persist sensitive resource") {
			t.Fatalf("%s error = %v", name, err)
		}
	}

	if err := s.Upsert("current-user", "normal", []byte(fmt.Sprintf(`{"id":%q,"first_name":"safe"}`, "normal"))); err != nil {
		t.Fatalf("safe current-user write rejected: %v", err)
	}
}
