// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Seeded-store behavioral tests for the fleet agents (degraded-site sweep)
// command, reusing the shared fleet test harness in fleet_offline_test.go.

package cli

import (
	"bytes"
	"path/filepath"
	"testing"
)

func TestFleetAgentsCommand(t *testing.T) {
	dbPath := seedFleetStore(t)
	flags := &rootFlags{asJSON: true}
	rows := runFleetJSONArray(t, newNovelFleetAgentsCmd(flags), dbPath)

	if len(rows) != 1 {
		t.Fatalf("fleet agents returned %d rows, want 1 (only the OFFLINE site)\nrows: %+v", len(rows), rows)
	}
	if got := rows[0]["site"]; got != "Beta Site" {
		t.Fatalf("degraded site = %v, want Beta Site", got)
	}
	if got := rows[0]["status"]; got != "OFFLINE" {
		t.Fatalf("status = %v, want OFFLINE", got)
	}
	for _, r := range rows {
		if r["site"] == "Acme HQ" {
			t.Fatal("ONLINE Acme HQ must not appear in the degraded-agent sweep")
		}
	}
}

func TestFleetAgentsEmptyStore(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "empty.db")
	flags := &rootFlags{asJSON: true}
	out := runFleetRaw(t, newNovelFleetAgentsCmd(flags), dbPath)
	// Empty store must yield an empty JSON array, not null and not an error.
	if got := string(bytes.TrimSpace(out)); got != "[]" {
		t.Fatalf("empty store fleet agents output = %q, want []", got)
	}
}

func TestFleetAgentsRejectsLiveDataSource(t *testing.T) {
	dbPath := seedFleetStore(t)
	flags := &rootFlags{asJSON: true, dataSource: "live"}
	cmd := newNovelFleetAgentsCmd(flags)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"--db", dbPath})
	if err := cmd.Execute(); err == nil {
		t.Fatal("fleet agents with --data-source live must error (local-only command)")
	}
}
