// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package store

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestSecretResourcesAreNeverPersisted(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	row := json.RawMessage(`{"id": 7, "name": "domain-admin", "params": {"password": "SENTINEL"}}`)
	if _, _, err := s.UpsertBatch("credentials", []json.RawMessage{row}); err != nil {
		t.Fatal(err)
	}
	if err := s.Upsert("integration_credentials", "7", row); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.UpsertBatch("companies", []json.RawMessage{json.RawMessage(`{"id": 1, "name": "Acme"}`)}); err != nil {
		t.Fatal(err)
	}
	var secret, kept int
	if err := s.DB().QueryRow(`SELECT COUNT(*) FROM resources WHERE resource_type IN ('credentials','integration_credentials')`).Scan(&secret); err != nil {
		t.Fatal(err)
	}
	if err := s.DB().QueryRow(`SELECT COUNT(*) FROM resources WHERE resource_type = 'companies'`).Scan(&kept); err != nil {
		t.Fatal(err)
	}
	if secret != 0 || kept != 1 {
		t.Fatalf("secret rows=%d (want 0), companies=%d (want 1)", secret, kept)
	}
}

// TestSecretTypedWritersAreGuarded covers the per-resource Upsert<X> entry
// points (sync's single-object path), which write a typed table besides the
// generic resources table. Each secret resource must leave both empty.
func TestSecretTypedWritersAreGuarded(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	row := json.RawMessage(`{"id": 3, "name": "job", "password": "SENTINEL", "params": {"password": "SENTINEL"}}`)
	for table, upsert := range map[string]func(json.RawMessage) error{
		"credentials":             s.UpsertCredentials,
		"integration_credentials": s.UpsertIntegrationCredentials,
		"scheduler":               s.UpsertScheduler,
		"attack_surface_results":  s.UpsertAttackSurfaceResults,
		"integration_rules":       s.UpsertIntegrationRules,
		"custom_domains":          s.UpsertCustomDomains,
	} {
		if err := upsert(row); err != nil {
			t.Fatalf("%s: %v", table, err)
		}
		var n int
		if err := s.DB().QueryRow(`SELECT COUNT(*) FROM "` + table + `"`).Scan(&n); err != nil {
			t.Fatalf("%s: %v", table, err)
		}
		if n != 0 {
			t.Fatalf("%s typed table holds %d row(s); secret rows must never be stored", table, n)
		}
	}
}
