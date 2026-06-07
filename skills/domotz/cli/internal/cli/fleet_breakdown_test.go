// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Seeded-store behavioral test for fleet breakdown.

package cli

import "testing"

func TestFleetBreakdownByVendor(t *testing.T) {
	dbPath := seedFleetStore(t)
	flags := &rootFlags{asJSON: true}
	rows := runFleetJSONArray(t, newNovelFleetBreakdownCmd(flags), dbPath, "--by", "vendor")

	// Three distinct vendors (Cisco, Axis, Ubiquiti), each count 1.
	if len(rows) != 3 {
		t.Fatalf("breakdown by vendor returned %d groups, want 3", len(rows))
	}
	for _, r := range rows {
		if cnt, ok := r["count"].(float64); !ok || cnt != 1 {
			t.Fatalf("group %v count = %v, want 1", r["vendor"], r["count"])
		}
	}
}

func TestFleetBreakdownByType(t *testing.T) {
	dbPath := seedFleetStore(t)
	flags := &rootFlags{asJSON: true}
	rows := runFleetJSONArray(t, newNovelFleetBreakdownCmd(flags), dbPath, "--by", "type")
	if len(rows) != 3 {
		t.Fatalf("breakdown by type returned %d groups, want 3 (Switch/Camera/Router)", len(rows))
	}
}

func TestFleetBreakdownInvalidBy(t *testing.T) {
	dbPath := seedFleetStore(t)
	flags := &rootFlags{asJSON: true}
	cmd := newNovelFleetBreakdownCmd(flags)
	cmd.SetArgs([]string{"--db", dbPath, "--by", "bogus"})
	cmd.SetOut(&nullWriter{})
	cmd.SetErr(&nullWriter{})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error for invalid --by value, got nil")
	}
}

type nullWriter struct{}

func (nullWriter) Write(p []byte) (int, error) { return len(p), nil }
