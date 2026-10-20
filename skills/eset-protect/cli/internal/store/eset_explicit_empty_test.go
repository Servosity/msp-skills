// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package store

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

// TestExplicitEmptyReplacesMirroredValue (handfixes.json:
// explicit-empty-replaces): the keep-richer merge kept a device's old
// activeProducts when a re-sync returned activeProducts: [], so fleet health
// and fleet gaps still counted a newly unprotected device as protected. A
// PRESENT empty value replaces; an OMITTED key still keeps the stored value.
func TestExplicitEmptyReplacesMirroredValue(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	const id = "00000000-0000-0000-0000-000000000001"
	put := func(body string) map[string]any {
		t.Helper()
		if err := s.Upsert("devices", id, json.RawMessage(body)); err != nil {
			t.Fatal(err)
		}
		raw, err := s.Get("devices", id)
		if err != nil {
			t.Fatal(err)
		}
		var obj map[string]any
		if err := json.Unmarshal(raw, &obj); err != nil {
			t.Fatal(err)
		}
		return obj
	}
	put(`{"uuid":"` + id + `","displayName":"PC","activeProducts":[{"name":"ESET Endpoint Security"}],"deployedComponents":[{"name":"agent"}],"tags":{"a":"b"},"note":"x"}`)
	got := put(`{"uuid":"` + id + `","displayName":"PC","activeProducts":[],"tags":{},"note":""}`)
	if p, _ := got["activeProducts"].([]any); len(p) != 0 {
		t.Fatalf("an explicit empty activeProducts kept the old products: %v", got["activeProducts"])
	}
	if m, _ := got["tags"].(map[string]any); len(m) != 0 {
		t.Fatalf("an explicit empty object kept old entries: %v", got["tags"])
	}
	if got["note"] != "" {
		t.Fatalf("an explicit empty string kept the old value: %v", got["note"])
	}
	if c, _ := got["deployedComponents"].([]any); len(c) != 1 {
		t.Fatalf("an omitted key must keep the stored value: %v", got["deployedComponents"])
	}
}
