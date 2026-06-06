// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"testing"
)

// TestPullViewEnvelope locks the stable JSON envelope pull emits: known row
// shapes land in rows/row_count, unknown shapes are preserved raw under data.
func TestPullViewEnvelope(t *testing.T) {
	known := json.RawMessage(`{"records":[{"a":1},{"a":2}]}`)
	view := pullView{ResourceID: "1", ResourceType: "dataset", Page: 1, PageSize: 50}
	if rows, ok := extractRows(known); ok {
		view.Rows = rows
		view.RowCount = len(rows)
	} else {
		view.Data = known
	}
	if view.RowCount != 2 || view.Data != nil {
		t.Fatalf("known envelope: %+v", view)
	}

	unknown := json.RawMessage(`{"chartConfig":{"series":[]}}`)
	view2 := pullView{ResourceID: "2", ResourceType: "widget", Page: 1, PageSize: 50}
	if rows, ok := extractRows(unknown); ok {
		view2.Rows = rows
		view2.RowCount = len(rows)
	} else {
		view2.Data = unknown
	}
	if view2.Data == nil || view2.RowCount != 0 {
		t.Fatalf("unknown envelope must be preserved raw: %+v", view2)
	}

	// The envelope must marshal with stable field names agents can --select.
	out, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"resource_id", "resource_type", "page", "page_size", "row_count", "rows"} {
		if _, ok := m[key]; !ok {
			t.Fatalf("envelope missing %q: %s", key, out)
		}
	}
}

func TestPullCommandShape(t *testing.T) {
	flags := &rootFlags{}
	cmd := newNovelPullCmd(flags)
	for _, f := range []string{"where", "page", "page-size", "type", "db"} {
		if cmd.Flags().Lookup(f) == nil {
			t.Fatalf("pull is missing --%s", f)
		}
	}
	if cmd.Annotations["mcp:read-only"] != "true" {
		t.Fatal("pull must be marked mcp:read-only")
	}
}
