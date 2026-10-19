// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package store

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

// A re-sync (sla present) replaces the row so ended SLA cycles disappear; a
// live read without sla merges so it cannot wipe synced SLA data.
func TestRequestRowsReplaceOnlyWithSLA(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	put := func(body string) {
		if err := s.Upsert("request", "1001", json.RawMessage(body)); err != nil {
			t.Fatal(err)
		}
	}
	read := func() map[string]any {
		var data string
		if err := s.DB().QueryRow(`SELECT data FROM resources WHERE resource_type='request' AND id='1001'`).Scan(&data); err != nil {
			t.Fatal(err)
		}
		var m map[string]any
		_ = json.Unmarshal([]byte(data), &m)
		return m
	}
	put(`{"issueId":"1001","sla":{"values":[{"name":"TTR","ongoingCycle":{"breached":true}}]},"old":"x"}`)
	put(`{"issueId":"1001","sla":{"values":[{"name":"TTR","completedCycles":[]}]}}`)
	if m := read(); m["old"] != nil {
		t.Fatalf("sync write must replace the row, got %v", m)
	}
	put(`{"issueId":"1001","summary":"live get"}`)
	if m := read(); m["sla"] == nil || m["summary"] != "live get" {
		t.Fatalf("live write without sla must merge and keep sla, got %v", m)
	}
}
