// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Seeded-store behavioral tests for the store-backed fleet commands. A real
// temp SQLite store is populated with two agents and three devices, then each
// command is executed and its JSON output asserted.

package cli

import (
	"bytes"
	"context"
	"domotz-pp-cli/internal/store"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
)

// seedFleetStore creates a temp store with a representative fleet and returns
// its path. Two agents (one ONLINE, one OFFLINE); three devices (one ONLINE,
// one OFFLINE, one DOWN) split across them; two alert profiles.
func seedFleetStore(t *testing.T) string {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "data.db")
	db, err := store.OpenWithContext(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer db.Close()

	agents := []string{
		`{"id":100,"display_name":"Acme HQ","status":{"value":"ONLINE"}}`,
		`{"id":200,"display_name":"Beta Site","status":{"value":"OFFLINE"}}`,
	}
	for _, a := range agents {
		if err := db.UpsertAgent(json.RawMessage(a)); err != nil {
			t.Fatalf("seed agent: %v", err)
		}
	}

	devices := []string{
		`{"id":1,"agent_id":"100","display_name":"Core Switch","status":"ONLINE","importance":"VITAL","type":{"label":"Switch"},"user_data":{"vendor":"Cisco","model":"C9300"},"os":{"name":"IOS-XE"},"first_seen_on":"2026-01-01T00:00:00Z"}`,
		`{"id":2,"agent_id":"100","display_name":"Lobby Cam","status":"OFFLINE","importance":"FLOATING","type":{"label":"Camera"},"user_data":{"vendor":"Axis","model":"P3245"},"os":{"name":""},"first_seen_on":"2026-05-30T06:00:00Z"}`,
		`{"id":3,"agent_id":"200","display_name":"Edge Router","status":"DOWN","importance":"VITAL","type":{"label":"Router"},"user_data":{"vendor":"Ubiquiti","model":"UDM"},"os":{"name":"UniFiOS"},"first_seen_on":"2026-05-30T07:00:00Z"}`,
	}
	for _, d := range devices {
		if err := db.UpsertDevice(json.RawMessage(d)); err != nil {
			t.Fatalf("seed device: %v", err)
		}
	}

	alerts := []string{
		`{"id":10,"alert_profile_id":10,"name":"Down Alert","description":"device down","is_enabled":true,"tag":"critical","device_id":1}`,
		`{"id":11,"alert_profile_id":11,"name":"Quiet Rule","description":"muted","is_enabled":false,"tag":"info","device_id":2}`,
	}
	for _, a := range alerts {
		if err := db.UpsertAlertProfile(json.RawMessage(a)); err != nil {
			t.Fatalf("seed alert_profile: %v", err)
		}
	}
	return dbPath
}

// runFleetJSONArray executes a leaf command with --json --db and decodes the
// stdout as a JSON array of objects.
func runFleetJSONArray(t *testing.T, cmd *cobra.Command, dbPath string, extra ...string) []map[string]any {
	t.Helper()
	out := runFleetRaw(t, cmd, dbPath, extra...)
	var rows []map[string]any
	if err := json.Unmarshal(out, &rows); err != nil {
		t.Fatalf("decode array from %q: %v\noutput: %s", cmd.Name(), err, out)
	}
	return rows
}

func runFleetRaw(t *testing.T, cmd *cobra.Command, dbPath string, extra ...string) []byte {
	t.Helper()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs(append([]string{"--db", dbPath}, extra...))
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute %q: %v", cmd.Name(), err)
	}
	return buf.Bytes()
}

func TestFleetOfflineCommand(t *testing.T) {
	dbPath := seedFleetStore(t)
	flags := &rootFlags{asJSON: true}
	rows := runFleetJSONArray(t, newNovelFleetOfflineCmd(flags), dbPath)

	if len(rows) != 2 {
		t.Fatalf("offline returned %d rows, want 2 (OFFLINE + DOWN)\nrows: %+v", len(rows), rows)
	}
	// VITAL (Edge Router, DOWN) must sort before FLOATING (Lobby Cam, OFFLINE).
	if got := rows[0]["display_name"]; got != "Edge Router" {
		t.Fatalf("first offline device = %v, want Edge Router (VITAL first)", got)
	}
	for _, r := range rows {
		if s := r["status"]; s != "OFFLINE" && s != "DOWN" {
			t.Fatalf("offline row has non-offline status %v", s)
		}
		if r["display_name"] == "Core Switch" {
			t.Fatal("ONLINE Core Switch must not appear in offline list")
		}
	}
}

func TestFleetOfflineEmptyStore(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "empty.db")
	flags := &rootFlags{asJSON: true}
	out := runFleetRaw(t, newNovelFleetOfflineCmd(flags), dbPath)
	// Empty store must yield an empty JSON array, not null and not an error.
	got := string(bytes.TrimSpace(out))
	if got != "[]" {
		t.Fatalf("empty store offline output = %q, want []", got)
	}
}
