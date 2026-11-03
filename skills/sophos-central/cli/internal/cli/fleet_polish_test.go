// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written: regression tests for the polish pass on the fleet reads.

package cli

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"sophos-central-pp-cli/internal/cliutil/testenv"
)

// A store file that does not exist yet must read the same as a store no
// refresh has filled: an empty result that carries the never-refreshed note,
// so an agent cannot mistake it for an all-clear.
func TestFleetReadsWithoutStoreCarryNote(t *testing.T) {
	testenv.Isolate(t)
	t.Setenv(sophosTenantEnv, "")
	sophosTenantFlag = ""
	db := filepath.Join(t.TempDir(), "missing.db")
	for _, sub := range []string{"triage", "health", "gaps"} {
		out, err := runRoot(t, "fleet", sub, "--db", db, "--json")
		if err != nil {
			t.Fatalf("fleet %s without a store: %v\n%s", sub, err, out)
		}
		var got struct {
			Items    []json.RawMessage `json:"items"`
			SyncedAt *time.Time        `json:"synced_at"`
			Note     string            `json:"note"`
		}
		if err := json.Unmarshal([]byte(out), &got); err != nil {
			t.Fatalf("fleet %s output is not JSON: %v\n%s", sub, err, out)
		}
		if len(got.Items) != 0 || got.SyncedAt != nil || got.Note != fleetNeverRefreshedNote {
			t.Fatalf("fleet %s without a store = %+v, want empty items, null synced_at and the never-refreshed note", sub, got)
		}
	}
}

// Passing --stale explicitly lists stale (low) devices even when --severity
// leaves low out, and admits no other low-severity rows.
func TestBuildTriageExplicitStaleOverridesSeverity(t *testing.T) {
	view, err := buildTriage(fleetFixture(), map[string]bool{"high": true, "medium": true}, 14*24*time.Hour, true, testNow, nil)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, r := range view.Items {
		if r.Kind == "stale" && r.ID == "E2" {
			found = true
		}
		if r.Severity == "low" && r.Kind != "stale" {
			t.Fatalf("explicit --stale let a non-stale low row through: %+v", r)
		}
	}
	if !found {
		t.Fatalf("explicit --stale must list stale endpoint E2 despite --severity high,medium: %+v", view.Items)
	}
}
