// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-fix regression test (handfixes.json: action-response-stored-on-parent).

package store

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

// The secret-key generate/rotate and api-keys revoke write-throughs hand
// UpsertBatch the parent record under the child partition name. It must land
// on the parent partition with no typed-table failure (the failure count is
// what prints "typed-table upsert failed" after a successful call), and the
// secret must still be scrubbed.
func TestActionResponseStoredOnParent(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	count := func(rt string) int {
		t.Helper()
		var n int
		if err := s.db.QueryRow(`SELECT COUNT(*) FROM resources WHERE resource_type = ?`, rt).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	data := func(rt, id string) string {
		t.Helper()
		var d string
		if err := s.db.QueryRow(`SELECT data FROM resources WHERE resource_type = ? AND id = ?`, rt, id).Scan(&d); err != nil {
			t.Fatalf("%s/%s: %v", rt, id, err)
		}
		return d
	}

	// A synced network and api key already in the mirror.
	if _, _, err := s.UpsertBatch("networks", []json.RawMessage{json.RawMessage(`{"id":"100","type":"networks","attributes":{"name":"HQ","physical_address":"1 Main St"}}`)}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.UpsertBatch("api-keys", []json.RawMessage{json.RawMessage(`{"id":"4","type":"api_keys","attributes":{"name":"ci","status":"active","last_four":"z123"}}`)}); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		partition, parent, id, item, want string
	}{
		{"secret_key", "networks", "100", `{"id":"100","type":"networks","attributes":{"name":"HQ","secret_key":"` + secretSentinel + `"}}`, `"physical_address":"1 Main St"`},
		{"secret_key", "networks", "101", `{"id":"101","type":"network","attributes":{"name":"Annex","secret_key":"` + secretSentinel + `"}}`, `"Annex"`},
		{"revoke", "api-keys", "4", `{"id":"4","type":"api_keys","attributes":{"status":"revoked","token":"***z123"}}`, `"status":"revoked"`},
	}
	for _, c := range cases {
		stored, extractFailures, typedFailures, err := s.UpsertBatchDetailed(c.partition, []json.RawMessage{json.RawMessage(c.item)})
		if err != nil || stored != 1 || extractFailures != 0 || typedFailures != 0 {
			t.Fatalf("%s write-through: stored=%d extractFailures=%d typedFailures=%d err=%v", c.partition, stored, extractFailures, typedFailures, err)
		}
		got := data(c.parent, c.id)
		if !strings.Contains(got, c.want) || strings.Contains(got, secretSentinel) || strings.Contains(got, `"token"`) {
			t.Fatalf("%s/%s after %s write-through: %s", c.parent, c.id, c.partition, got)
		}
		var typed int
		if err := s.db.QueryRow(`SELECT COUNT(*) FROM "`+strings.ReplaceAll(c.parent, "-", "_")+`" WHERE id = ?`, c.id).Scan(&typed); err != nil || typed != 1 {
			t.Fatalf("typed %s row for %s: count=%d err=%v", c.parent, c.id, typed, err)
		}
	}
	if n := count("secret_key"); n != 0 {
		t.Fatalf("%d network rows were stored under secret_key", n)
	}
	if n := count("revoke"); n != 0 {
		t.Fatalf("%d api key rows were stored under revoke", n)
	}

	// A row that does not declare the parent type keeps the generated
	// behavior: it stays on the child partition.
	if stored, _, _, err := s.UpsertBatchDetailed("revoke", []json.RawMessage{json.RawMessage(`{"id":"r1","api_keys_id":"4"}`)}); err != nil || stored != 1 {
		t.Fatalf("child row: stored=%d err=%v", stored, err)
	}
	if n := count("revoke"); n != 1 {
		t.Fatalf("child row was moved off revoke (count=%d)", n)
	}
}
