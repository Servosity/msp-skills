// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Seeded-store behavioral tests for the fleet unmonitored (coverage-gap)
// command. Extends the shared fleet seed with devices carrying explicit
// authentication_status / snmp_status values across the gap and healthy
// enums.

package cli

import (
	"bytes"
	"context"
	"domotz-pp-cli/internal/store"
	"encoding/json"
	"path/filepath"
	"testing"
)

// seedCoverageStore seeds agents plus devices spanning every coverage state:
// two with auth gaps, one with an SNMP gap, and two healthy.
func seedCoverageStore(t *testing.T) string {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "coverage.db")
	db, err := store.OpenWithContext(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer db.Close()

	if err := db.UpsertAgent(json.RawMessage(`{"id":100,"display_name":"Acme HQ","status":{"value":"ONLINE"}}`)); err != nil {
		t.Fatalf("seed agent: %v", err)
	}
	devices := []string{
		`{"id":1,"agent_id":"100","display_name":"NAS","authentication_status":"WRONG_CREDENTIALS","snmp_status":"AUTHENTICATED","importance":"VITAL","type":{"label":"Storage"}}`,
		`{"id":2,"agent_id":"100","display_name":"Firewall","authentication_status":"REQUIRED","snmp_status":null,"importance":"FLOATING","type":{"label":"Firewall"}}`,
		`{"id":3,"agent_id":"100","display_name":"Core Switch","authentication_status":"AUTHENTICATED","snmp_status":"NOT_AUTHENTICATED","importance":"VITAL","type":{"label":"Switch"}}`,
		`{"id":4,"agent_id":"100","display_name":"Printer","authentication_status":"NO_AUTHENTICATION","snmp_status":"NOT_FOUND","importance":"FLOATING","type":{"label":"Printer"}}`,
		`{"id":5,"agent_id":"100","display_name":"AP","authentication_status":"AUTHENTICATED","snmp_status":"AUTHENTICATED","importance":"FLOATING","type":{"label":"AP"}}`,
	}
	for _, d := range devices {
		if err := db.UpsertDevice(json.RawMessage(d)); err != nil {
			t.Fatalf("seed device: %v", err)
		}
	}
	return dbPath
}

func TestFleetUnmonitoredCommand(t *testing.T) {
	dbPath := seedCoverageStore(t)
	flags := &rootFlags{asJSON: true}
	rows := runFleetJSONArray(t, newNovelFleetUnmonitoredCmd(flags), dbPath)

	// Gaps: WRONG_CREDENTIALS (NAS), REQUIRED (Firewall), NOT_AUTHENTICATED snmp (Core Switch).
	// Healthy: NO_AUTHENTICATION/NOT_FOUND (Printer), AUTHENTICATED (AP).
	if len(rows) != 3 {
		t.Fatalf("unmonitored returned %d rows, want 3\nrows: %+v", len(rows), rows)
	}
	// VITAL rows sort first.
	if got := rows[0]["importance"]; got != "VITAL" {
		t.Fatalf("first row importance = %v, want VITAL", got)
	}
	for _, r := range rows {
		name := r["display_name"]
		if name == "Printer" || name == "AP" {
			t.Fatalf("healthy device %v must not appear in the coverage audit", name)
		}
	}
}

func TestFleetUnmonitoredEmptyStore(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "empty.db")
	flags := &rootFlags{asJSON: true}
	out := runFleetRaw(t, newNovelFleetUnmonitoredCmd(flags), dbPath)
	if got := string(bytes.TrimSpace(out)); got != "[]" {
		t.Fatalf("empty store unmonitored output = %q, want []", got)
	}
}
