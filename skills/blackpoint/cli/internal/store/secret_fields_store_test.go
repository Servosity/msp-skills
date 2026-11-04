// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written (secret-field-persistence class): a response secret (a dark-web
// exposure's password) never reaches the store file, by any write path.

package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStoreNeverPersistsAResponseSecretField(t *testing.T) {
	const pw = "Hunter2SentinelPW"
	dbPath := filepath.Join(t.TempDir(), "data.db")
	s, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	row := json.RawMessage(`{"id":"e1","username":"bob@acme.test","password":"` + pw + `","breachName":"x"}`)
	if _, _, err := s.UpsertBatch("darkweb-exposures", []json.RawMessage{row}); err != nil {
		t.Fatalf("batch upsert: %v", err)
	}
	if err := s.UpsertDarkwebExposures(row); err != nil {
		t.Fatalf("typed upsert: %v", err)
	}
	if err := s.Upsert("darkweb_exposures", "e2", row); err != nil {
		t.Fatalf("generic upsert: %v", err)
	}
	if _, err := s.db.Exec(`INSERT INTO resources (id, resource_type, data, synced_at, updated_at) VALUES ('raw','x',?,'','')`, string(row)); err == nil {
		t.Fatal("a raw write of a secret-bearing row must be refused by the driver")
	}
	_ = s.Close()
	for _, p := range []string{dbPath, dbPath + "-wal"} {
		data, _ := os.ReadFile(p)
		if strings.Contains(string(data), pw) {
			t.Fatalf("%s holds the response password", filepath.Base(p))
		}
	}
}

// TestStoreSeesEveryKeyOccurrence: a duplicate key, a second top-level value
// or text the decoder cannot parse hides a secret from the decoded value. No
// write path may store those bytes, and the driver refuses them raw. Plain
// column text that only starts like JSON is still written.
func TestStoreSeesEveryKeyOccurrence(t *testing.T) {
	const pw = "breached-secret-123"
	dbPath := filepath.Join(t.TempDir(), "data.db")
	s, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	dup := json.RawMessage(`{"id":"e1","username":"bob@acme.test","password":"` + pw + `","password":null,"breachName":"x"}`)
	if _, _, err := s.UpsertBatch("darkweb-exposures", []json.RawMessage{dup}); err != nil {
		t.Fatalf("batch upsert: %v", err)
	}
	if err := s.UpsertDarkwebExposures(dup); err != nil {
		t.Fatalf("typed upsert: %v", err)
	}
	if err := s.Upsert("darkweb_exposures", "e2", dup); err != nil {
		t.Fatalf("generic upsert: %v", err)
	}
	if got, err := s.Get("darkweb_exposures", "e2"); err != nil || strings.Contains(string(got), pw) || !strings.Contains(string(got), `"username":"bob@acme.test"`) {
		t.Errorf("the stored row must keep its plain fields and drop the secret: %s (%v)", got, err)
	}
	for id, v := range map[string]string{
		"e3": `{"id":"e3"} {"password":"` + pw + `"}`,
		"e4": `{"id":"e4"}garbage"password":"` + pw + `"`,
		"e5": `{"id":"e5","password":"` + pw + `"`,
		"e6": `{"id":"e6","password":"` + pw + `\q"}`,
		"e7": "\xEF\xBB\xBF" + `{"id":"e7","password":"` + pw + `"}`,
	} {
		_ = s.Upsert("darkweb_exposures", id, json.RawMessage(v))
	}
	for name, v := range map[string]string{
		"duplicate":    string(dup),
		"second-value": `{"id":"r"} {"password":"` + pw + `"}`,
		"trailing":     `{"id":"r"}garbage"password":"` + pw + `"`,
		"bad-escape":   `{"id":"r","password":"` + pw + `\q"}`,
	} {
		if _, err := s.db.Exec(`INSERT INTO resources (id, resource_type, data, synced_at, updated_at) VALUES (?,'x',?,'','')`, "raw-"+name, v); err == nil {
			t.Errorf("a raw %s write holding a secret must be refused by the driver", name)
		}
	}
	for _, v := range []string{"[TEST] Acme Dental", "[1] Acme", "{draft} notes", `{"id":"c","token_hint":"abc","keyboard":"us"}`, `{"password":null,"password":true}`} {
		if _, err := s.db.Exec(`INSERT INTO resources (id, resource_type, data, synced_at, updated_at) VALUES (?,'clean',?,'','')`, v, v); err != nil {
			t.Errorf("a clean value %q was refused: %v", v, err)
		}
	}
	_ = s.Close()
	for _, p := range []string{dbPath, dbPath + "-wal"} {
		data, _ := os.ReadFile(p)
		if strings.Contains(string(data), pw) {
			t.Fatalf("%s holds the response password", filepath.Base(p))
		}
	}
}
