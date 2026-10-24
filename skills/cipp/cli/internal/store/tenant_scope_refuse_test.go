// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/cipp/handfixes.json under the
// "fanout-save-integrity" entry.

package store

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

// TestReplaceTenantRowsRefusesNonRowItems: a response holding a CIPP failure
// string is not the tenant's data, so the stored rows must survive.
func TestReplaceTenantRowsRefusesNonRowItems(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	TenantScopeResource("policies")
	rows := []json.RawMessage{json.RawMessage(`{"id":"p1","_tenant":"t.example"}`), json.RawMessage(`{"id":"p2","_tenant":"t.example"}`)}
	if n, err := s.ReplaceTenantRows("policies", "t.example", rows); err != nil || n != 2 {
		t.Fatalf("seed: n=%d err=%v", n, err)
	}
	if _, err := s.ReplaceTenantRows("policies", "t.example", []json.RawMessage{json.RawMessage(`"Failed to retrieve policies"`)}); err == nil {
		t.Fatal("a failure string replaced the tenant's rows")
	}
	got, err := s.List("policies", 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("stored rows changed: %d left", len(got))
	}
}
