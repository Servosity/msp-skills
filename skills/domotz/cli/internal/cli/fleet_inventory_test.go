// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Seeded-store behavioral test for fleet inventory.

package cli

import "testing"

func TestFleetInventoryCommand(t *testing.T) {
	dbPath := seedFleetStore(t)
	flags := &rootFlags{asJSON: true}
	rows := runFleetJSONArray(t, newNovelFleetInventoryCmd(flags), dbPath)

	if len(rows) != 3 {
		t.Fatalf("inventory returned %d rows, want 3", len(rows))
	}
	// Find the Core Switch and check its enriched columns.
	var found bool
	for _, r := range rows {
		if r["display_name"] == "Core Switch" {
			found = true
			if r["vendor"] != "Cisco" {
				t.Fatalf("Core Switch vendor = %v, want Cisco", r["vendor"])
			}
			if r["model"] != "C9300" {
				t.Fatalf("Core Switch model = %v, want C9300", r["model"])
			}
			if r["site"] != "Acme HQ" {
				t.Fatalf("Core Switch site = %v, want Acme HQ", r["site"])
			}
			if r["type"] != "Switch" {
				t.Fatalf("Core Switch type = %v, want Switch", r["type"])
			}
		}
	}
	if !found {
		t.Fatal("Core Switch missing from inventory")
	}
}
