// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Class tests for the 4.34.2 reprint's credential fix (orchestrator group A):
// credential-persistence and credential-migration-fail-open.

package store

import (
	"path/filepath"
	"strings"
	"testing"
)

const legacySecret = "SmtpSecret55443322"

// legacyStore builds a store the way a build without credential redaction
// left it: vendor rows (generic blob and typed table) carrying an SMTP
// password, and no record that the store was written credential-free.
func legacyStore(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "data.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	doc := `{"domain":"acme.example","email-send-smtp-password":"` + legacySecret + `"}`
	for _, q := range []string{
		`INSERT INTO resources (id, resource_type, data) VALUES ('acme.example', 'domains', '` + doc + `')`,
		`INSERT INTO "domains" ("id", "data") VALUES ('acme.example', '` + doc + `')`,
		`DROP TABLE IF EXISTS netsapiens_store_meta`,
	} {
		if _, err := s.DB().Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	_ = s.Close()
	return path
}

// A store that may hold vendor credentials is refused on every open, read-only
// or not, with an explanation and a remedy, and nothing is migrated half-way.
func TestClassLegacyStoreIsRefusedNotMigrated(t *testing.T) {
	path := legacyStore(t)
	for name, open := range map[string]func(string) (*Store, error){"Open": Open, "OpenReadOnly": OpenReadOnly} {
		s, err := open(path)
		if err == nil {
			rows := 0
			_ = s.DB().QueryRow(`SELECT COUNT(*) FROM resources WHERE data LIKE '%` + legacySecret + `%'`).Scan(&rows)
			_ = s.Close()
			t.Fatalf("%s opened a store that may hold vendor credentials (%d row(s) still carry the SMTP password); it must refuse", name, rows)
		}
		msg := err.Error()
		if strings.Contains(msg, legacySecret) {
			t.Fatalf("%s's refusal repeats the credential: %s", name, msg)
		}
		if !strings.Contains(msg, "local-data purge") {
			t.Fatalf("%s's refusal must say how to recover: %s", name, msg)
		}
	}
}

// The gate is not a false RED: a store this build creates opens, takes rows
// and opens again, read-only too.
func TestClassFreshStoreOpens(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB().Exec(`INSERT INTO resources (id, resource_type, data) VALUES ('acme.example', 'domains', '{"domain":"acme.example"}')`); err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
	for name, open := range map[string]func(string) (*Store, error){"Open": Open, "OpenReadOnly": OpenReadOnly} {
		s, err := open(path)
		if err != nil {
			t.Fatalf("%s refused a store this build wrote: %v", name, err)
		}
		_ = s.Close()
	}
}
