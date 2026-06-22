// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-authored tests for the MCP `search` concise projection (issue #101
// residual item #2): the MCP search tool, like the CLI, must default to the
// id/name/type/match projection rather than dumping whole raw records.

package mcp

import (
	"encoding/json"
	"strings"
	"testing"

	"axcient-pp-cli/internal/store"
)

func TestProjectSearchHit_DeviceShape(t *testing.T) {
	hit := store.SearchHit{
		ResourceType: "device",
		Data:         json.RawMessage(`{"id_":7654321,"name":"Acme-FS01","devices_counters":{"nested":"WALL_OF_JSON"}}`),
	}
	row := projectSearchHit(hit, "Acme")
	// Numeric id_ must render as an integer, not scientific notation (1.234e+06).
	if row.ID != "7654321" {
		t.Fatalf("ID = %q, want 7654321", row.ID)
	}
	if row.Name != "Acme-FS01" {
		t.Fatalf("Name = %q, want Acme-FS01", row.Name)
	}
	if row.Type != "device" {
		t.Fatalf("Type = %q, want device", row.Type)
	}
	if row.Match != "name~Acme" {
		t.Fatalf("Match = %q, want name~Acme", row.Match)
	}

	// The projected row must NOT carry the raw nested blocks the tester reported
	// (devices_counters) — that's the whole point of the projection.
	out, err := json.Marshal(row)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(out); strings.Contains(got, "devices_counters") || strings.Contains(got, "WALL_OF_JSON") {
		t.Fatalf("projection leaked raw record fields: %s", got)
	}
}

func TestSearchMatchIndicator(t *testing.T) {
	obj := map[string]any{"name": "Acme-FS01", "note": "hello world"}
	if got := searchMatchIndicator(obj, "Acme"); got != "name~Acme" {
		t.Fatalf("searchMatchIndicator(Acme) = %q, want name~Acme", got)
	}
	if got := searchMatchIndicator(obj, "world"); got != "note~world" {
		t.Fatalf("searchMatchIndicator(world) = %q, want note~world", got)
	}
	if got := searchMatchIndicator(obj, "zzz"); got != "" {
		t.Fatalf("searchMatchIndicator(no-substring) = %q, want empty", got)
	}
}
