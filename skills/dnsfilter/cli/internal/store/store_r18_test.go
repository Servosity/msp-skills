// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-fix regression tests (handfixes.json: sync-non-object-item-is-incomplete,
// merge-explicit-empty-scalar-is-authoritative,
// typed-failure-drops-stale-projection), Codex round 18.

package store

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

// A map-keyed record the response names but carries nothing for ({} under its
// key) is an extract failure, never silently dropped: sync must not certify
// the walk complete and --full must not prune the stored row it names.
func TestMapKeyedEmptyRecordIsAnExtractFailure(t *testing.T) {
	items, ok := FlattenMapKeyedCollection(json.RawMessage(`{"1":{},"2":{"id":"2","type":"user_agents"}}`))
	if !ok {
		t.Fatal("the map-keyed collection was not recognized")
	}
	if len(items) != 2 {
		t.Fatalf("flattened %d items, want 2 (the empty record must reach integrity accounting): %s", len(items), items)
	}
	s, err := Open(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ids, stored, extractFailures, _, err := s.UpsertBatchStoredIDs("user-agents", items)
	if err != nil {
		t.Fatal(err)
	}
	if stored != 1 || extractFailures != 1 || len(ids) != 1 || ids[0] != "2" {
		t.Fatalf("stored=%d extractFailures=%d ids=%q, want 1 stored (2) and 1 extract failure", stored, extractFailures, ids)
	}
	// A nested bucket's empty record counts the same way.
	nested, ok := FlattenMapKeyedCollection(json.RawMessage(`{"900":{"1":{},"2":{"id":"2"}}}`))
	if !ok || len(nested) != 2 {
		t.Fatalf("nested bucket flattened to %d items (ok=%v), want 2", len(nested), ok)
	}
}

// A key the API sends replaces the stored value whatever its type: a list now
// sent as false (unreadable) is never hidden behind the stale list, while an
// omitted key (a thinner list response) still keeps the richer stored value.
func TestMergePresentKeyOfAnyTypeReplaces(t *testing.T) {
	existing := json.RawMessage(`{"id":"10","attributes":{"name":"Std","whitelist_domains":[],"blacklist_domains":["a.example"],"settings":{"x":1},"rels":[{"id":"1","name":"kept"}],"note":"n"}}`)
	for _, tc := range []struct{ key, incoming, want string }{
		{"blacklist_domains", `false`, `false`},
		{"blacklist_domains", `"oops"`, `"oops"`},
		{"blacklist_domains", `7`, `7`},
		{"settings", `true`, `true`},
		{"settings", `[]`, `[]`},
		{"note", `{}`, `{}`},
		{"note", `[]`, `[]`},
	} {
		incoming := json.RawMessage(`{"id":"10","attributes":{"name":"Std","` + tc.key + `":` + tc.incoming + `}}`)
		merged, ok := mergeKeepRicherJSON("policies", existing, incoming)
		if !ok {
			t.Fatal("mergeKeepRicherJSON returned !ok")
		}
		var got struct {
			Attributes map[string]json.RawMessage `json:"attributes"`
		}
		if err := json.Unmarshal(merged, &got); err != nil {
			t.Fatal(err)
		}
		if v := string(got.Attributes[tc.key]); v != tc.want {
			t.Errorf("%s sent as %s merged to %s, want the sent value: %s", tc.key, tc.incoming, v, merged)
		}
		// Every key the incoming row omitted keeps its stored value.
		for _, k := range []string{"whitelist_domains", "rels"} {
			if _, present := got.Attributes[k]; !present {
				t.Errorf("omitted %s was dropped: %s", k, merged)
			}
		}
	}
	// List-vs-detail thinness: a list row that omits keys keeps the detail.
	thin := json.RawMessage(`{"id":"10","attributes":{"name":"Std"}}`)
	merged, _ := mergeKeepRicherJSON("policies", existing, thin)
	for _, want := range []string{`"blacklist_domains":["a.example"]`, `"settings":{"x":1}`, `"rels":[{"id":"1","name":"kept"}]`} {
		if !strings.Contains(string(merged), want) {
			t.Errorf("a thinner row lost %s: %s", want, merged)
		}
	}
	// Through the store: the batch writer stores the sent false.
	s, err := Open(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, _, err := s.UpsertBatch("policies", []json.RawMessage{existing}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.UpsertBatch("policies", []json.RawMessage{json.RawMessage(`{"id":"10","attributes":{"name":"Std","blacklist_domains":false}}`)}); err != nil {
		t.Fatal(err)
	}
	row, err := s.Get("policies", "10")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(row), `"blacklist_domains":false`) {
		t.Fatalf("the stored row hides the sent false behind the stale list: %s", row)
	}
}

// A typed projection that fails while its generic row commits (a NOT NULL
// parent key sent as null) never leaves the earlier typed row behind with
// equal counts: the typed partition is incomplete, so typed reads fall back
// to the generic rows, which hold the new value.
func TestFailedTypedUpdateMakesThePartitionIncomplete(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	first := json.RawMessage(`{"id":"1","type":"lan_ips","parent_id":"2","networks_id":"2","attributes":{"ip":"10.0.0.1"}}`)
	if _, _, typed, err := s.UpsertBatchDetailed("lan_ips", []json.RawMessage{first}); err != nil || typed != 0 {
		t.Fatalf("first write: typed failures=%d err=%v", typed, err)
	}
	if complete, _, err := s.TypedPartitionComplete("lan_ips"); err != nil || !complete {
		t.Fatalf("after a clean write the typed partition is complete=%v err=%v, want complete", complete, err)
	}
	second := json.RawMessage(`{"id":"1","type":"lan_ips","parent_id":"2","networks_id":null,"attributes":{"ip":"10.0.0.9"}}`)
	_, _, typed, err := s.UpsertBatchDetailed("lan_ips", []json.RawMessage{second})
	if err != nil {
		t.Fatal(err)
	}
	if typed != 1 {
		t.Fatalf("the fixture did not fail its typed projection (typed failures=%d)", typed)
	}
	if complete, _, err := s.TypedPartitionComplete("lan_ips"); err != nil || complete {
		t.Fatalf("a failed typed update left the typed partition complete=%v (err=%v): typed reads return the stale IP", complete, err)
	}
	if rows, ok, err := s.ListTypedRange("lan_ips", 10, 0); err != nil || ok {
		t.Fatalf("ListTypedRange served the typed table (ok=%v, err=%v): %s", ok, err, rows)
	}
	generic, err := s.List("lan_ips", 10)
	if err != nil || len(generic) != 1 || !strings.Contains(string(generic[0]), "10.0.0.9") {
		t.Fatalf("the generic rows do not hold the new value (err=%v): %s", err, generic)
	}
	// A later clean write projects the row again.
	third := json.RawMessage(`{"id":"1","type":"lan_ips","parent_id":"2","networks_id":"2","attributes":{"ip":"10.0.0.9"}}`)
	if _, _, typed, err := s.UpsertBatchDetailed("lan_ips", []json.RawMessage{third}); err != nil || typed != 0 {
		t.Fatalf("third write: typed failures=%d err=%v", typed, err)
	}
	if complete, _, _ := s.TypedPartitionComplete("lan_ips"); !complete {
		t.Fatal("a clean write after the failure did not make the typed partition complete again")
	}
}
