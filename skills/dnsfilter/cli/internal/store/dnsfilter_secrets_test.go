// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-fix regression tests (handfixes.json: secrets-never-persisted).

package store

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

const secretSentinel = "SENTINEL-SECRET-123"

func TestIsSecretKey(t *testing.T) {
	for _, k := range []string{
		"secret_key", "token", "client_secret", "access_token", "payment_token",
		"ep-grd-subscriber-secret", "new_password", "reset_password_url",
		"private_key", "X-Api-Key", "apiKey", "refresh_token", "Password",
		"preshared_key", "credentials",
	} {
		if !IsSecretKey(k) {
			t.Errorf("IsSecretKey(%q) = false, want true", k)
		}
	}
	// Every field a novel command reads from the mirror, plus near misses.
	for _, k := range []string{
		"token_type", "last_four", "name", "hostname", "agent_state", "agent_type",
		"agent_version", "last_sync", "uninstalled_at", "whitelist_domains",
		"blacklist_domains", "blacklist_categories", "organization_id", "policy_ids",
		"relationships", "attributes", "id", "type", "hashed_device_sn", "friendly_name",
		"youtube_restricted_level", "scheduled_policy", "network", "policy", "status",
		"key", "", "api_keys_id", "apiKeyId", "secret_key_id", "token_ids", "networks_id", "total_tokens", "max_tokens",
	} {
		if IsSecretKey(k) {
			t.Errorf("IsSecretKey(%q) = true, want false", k)
		}
	}
}

func TestScrubPersistedSecrets(t *testing.T) {
	in := json.RawMessage(`{"id":"7","type":"networks","attributes":{"name":"HQ <a&b>","secret_key":"` + secretSentinel + `","big":12345678901234567890,` +
		`"has_secret":true,"password":null,"nested":{"client_secret":"` + secretSentinel + `"},` +
		`"list":[{"access_token":"` + secretSentinel + `","keep":1},"plain"],"Token":{"value":"` + secretSentinel + `"}}}`)
	out := ScrubPersistedSecrets(in)
	if bytes.Contains(bytes.ToLower(out), []byte("sentinel")) {
		t.Fatalf("secret survived the scrub: %s", out)
	}
	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("scrubbed row is not JSON: %v (%s)", err, out)
	}
	attrs := got["attributes"].(map[string]any)
	if attrs["name"] != "HQ <a&b>" || attrs["has_secret"] != true {
		t.Fatalf("non-secret fields were changed: %s", out)
	}
	if v, ok := attrs["password"]; !ok || v != nil {
		t.Fatalf("a null secret-named flag should be kept: %s", out)
	}
	if !bytes.Contains(out, []byte(`12345678901234567890`)) || !bytes.Contains(out, []byte(`<a&b>`)) {
		t.Fatalf("number precision or HTML characters were rewritten: %s", out)
	}
	if list := attrs["list"].([]any); len(list) != 2 || list[1] != "plain" {
		t.Fatalf("array items were lost: %s", out)
	}

	clean := json.RawMessage(`{"id":"7", "attributes":{"name":"HQ","n":1.50}}`)
	if got := ScrubPersistedSecrets(clean); !bytes.Equal(got, clean) {
		t.Fatalf("a row with no secret must be returned byte-for-byte: %s", got)
	}
	for _, bad := range []string{`{"secret_key":"` + secretSentinel, `{"a":1} {"secret_key":"` + secretSentinel + `"}`, `not json ` + secretSentinel} {
		if got := ScrubPersistedSecrets(json.RawMessage(bad)); string(got) != `{}` {
			t.Fatalf("undecodable input must be withheld whole, got %s", got)
		}
	}
}

// secretPayload is a JSON:API row carrying secrets at several depths plus
// every NOT NULL column any typed table declares, so each typed writer
// actually commits its row.
func secretPayload(t *testing.T, s *Store, id string) json.RawMessage {
	t.Helper()
	row := map[string]any{
		"id":   id,
		"type": "networks",
		"attributes": map[string]any{
			"name":          "VISIBLE-MARKER-" + id,
			"secret_key":    secretSentinel,
			"token":         secretSentinel,
			"last_four":     "z123",
			"nested":        map[string]any{"client_secret": secretSentinel},
			"list":          []any{map[string]any{"access_token": secretSentinel}},
			"payment_token": secretSentinel,
		},
		"secret_key": secretSentinel,
	}
	for _, col := range notNullColumns(t, s) {
		if _, ok := row[col]; !ok {
			row[col] = "p1"
		}
	}
	data, err := json.Marshal(row)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func notNullColumns(t *testing.T, s *Store) []string {
	t.Helper()
	rows, err := s.db.Query(`SELECT name FROM sqlite_master WHERE type = 'table'`)
	if err != nil {
		t.Fatal(err)
	}
	var tables []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			t.Fatal(err)
		}
		tables = append(tables, n)
	}
	rows.Close()
	seen := map[string]bool{}
	for _, table := range tables {
		cols, err := s.db.Query(fmt.Sprintf(`SELECT name, "notnull" FROM pragma_table_info('%s')`, strings.ReplaceAll(table, "'", "''")))
		if err != nil {
			continue
		}
		for cols.Next() {
			var name string
			var notNull int
			if err := cols.Scan(&name, &notNull); err != nil {
				t.Fatal(err)
			}
			if notNull == 1 && name != "id" && name != "data" && name != "synced_at" {
				seen[name] = true
			}
		}
		cols.Close()
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// grepDir reports every file under dir whose bytes contain needle
// (case-insensitive).
func grepDir(t *testing.T, dir, needle string) []string {
	t.Helper()
	var hits []string
	needle = strings.ToLower(needle)
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if bytes.Contains(bytes.ToLower(b), []byte(needle)) {
			hits = append(hits, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return hits
}

// TestSecretsNeverReachTheStoreFile drives every write path (the generic
// Upsert, every typed Upsert<X>, UpsertBatch for every typed resource and an
// untyped one, and a merge over an existing row) with secret-bearing rows,
// then greps the raw SQLite file, WAL and SHM for the sentinel.
func TestSecretsNeverReachTheStoreFile(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	closed := false
	defer func() {
		if !closed {
			s.Close()
		}
	}()

	n := 0
	nextID := func() string { n++; return fmt.Sprintf("%d", 1000+n) }

	// Every typed writer behind Upsert<X>, found by reflection so a writer
	// added by a regen is covered without editing this test.
	rawType := reflect.TypeOf(json.RawMessage(nil))
	errType := reflect.TypeOf((*error)(nil)).Elem()
	sv := reflect.ValueOf(s)
	typedWriters := 0
	for i := 0; i < sv.NumMethod(); i++ {
		m := sv.Type().Method(i)
		ft := m.Type
		if !strings.HasPrefix(m.Name, "Upsert") || ft.NumIn() != 2 || ft.In(1) != rawType || ft.NumOut() != 1 || ft.Out(0) != errType {
			continue
		}
		out := sv.Method(i).Call([]reflect.Value{reflect.ValueOf(secretPayload(t, s, nextID()))})
		if e, _ := out[0].Interface().(error); e != nil {
			t.Fatalf("%s: %v", m.Name, e)
		}
		typedWriters++
	}
	if typedWriters < len(typedListTableByResource) {
		t.Fatalf("found %d typed Upsert<X> writers, want at least %d", typedWriters, len(typedListTableByResource))
	}

	// UpsertBatch (sync and the live/mutation write-through) for every typed
	// resource, plus an untyped one; each typed table must hold the row.
	types := make([]string, 0, len(typedListTableByResource)+1)
	for rt := range typedListTableByResource {
		types = append(types, rt)
	}
	sort.Strings(types)
	types = append(types, "networks-geo")
	for _, rt := range types {
		stored, _, typedFailures, err := s.UpsertBatchDetailed(rt, []json.RawMessage{secretPayload(t, s, nextID())})
		if err != nil || stored != 1 || typedFailures != 0 {
			t.Fatalf("UpsertBatch %s: stored=%d typedFailures=%d err=%v", rt, stored, typedFailures, err)
		}
	}
	for rt, table := range typedListTableByResource {
		var c int
		if err := s.db.QueryRow(fmt.Sprintf(`SELECT COUNT(*) FROM "%s"`, table)).Scan(&c); err != nil || c == 0 {
			t.Fatalf("typed table %s for %s holds no row (count=%d err=%v); the writer was not exercised", table, rt, c, err)
		}
	}

	// Generic Upsert, then a merge over a stored row (both directions).
	if err := s.Upsert("networks-subnets", "g1", secretPayload(t, s, "g1")); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertNetworks(json.RawMessage(`{"id":"m1","type":"networks","attributes":{"name":"VISIBLE-MARKER-m1"}}`)); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertNetworks(json.RawMessage(`{"id":"m1","type":"networks","attributes":{"secret_key":"` + secretSentinel + `"}}`)); err != nil {
		t.Fatal(err)
	}

	// While open, rows live in the WAL; prove the grep sees them.
	if hits := grepDir(t, dir, "VISIBLE-MARKER-m1"); len(hits) == 0 {
		t.Fatal("visible marker not found in the store files; the grep is not looking where rows land")
	}
	if hits := grepDir(t, dir, "sentinel"); len(hits) > 0 {
		t.Fatalf("secret sentinel persisted in %v", hits)
	}
	var fts int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM resources_fts WHERE resources_fts MATCH 'sentinel'`).Scan(&fts); err != nil || fts != 0 {
		t.Fatalf("FTS index matches the sentinel (count=%d err=%v)", fts, err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	closed = true
	if hits := grepDir(t, dir, "sentinel"); len(hits) > 0 {
		t.Fatalf("secret sentinel persisted after checkpoint in %v", hits)
	}
}

// TestMergeDropsALegacySecret: a secret already in a stored row (a store
// written before this fix) is dropped by the next write of that row instead
// of being carried over by the keep-richer merge.
func TestMergeDropsALegacySecret(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.db.Exec(`INSERT INTO resources (id, resource_type, data) VALUES ('m2', 'networks', ?)`,
		`{"id":"m2","attributes":{"name":"HQ","secret_key":"`+secretSentinel+`"}}`); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertNetworks(json.RawMessage(`{"id":"m2","attributes":{"name":"HQ2"}}`)); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{`SELECT data FROM resources WHERE resource_type = 'networks' AND id = 'm2'`, `SELECT data FROM "networks" WHERE id = 'm2'`} {
		var data string
		if err := s.db.QueryRow(q).Scan(&data); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(data, secretSentinel) || !strings.Contains(data, "HQ2") {
			t.Fatalf("%s: legacy secret carried over or row not updated: %s", q, data)
		}
	}
}
