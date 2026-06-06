// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"testing"
)

// marshalJSONArray must never emit JSON null: insights.groupLite uses pointer
// slices to distinguish "associations never synced" (key absent/null) from
// "genuinely zero members/owners" ([]). A null for an ownerless group would
// silently drop the exact finding 'groups risk' exists to surface.
func TestMarshalJSONArrayNeverNull(t *testing.T) {
	cases := []struct {
		name  string
		items []json.RawMessage
		want  string
	}{
		{"nil slice", nil, "[]"},
		{"empty slice", []json.RawMessage{}, "[]"},
		{"one element", []json.RawMessage{json.RawMessage(`{"id":"x"}`)}, `[{"id":"x"}]`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := marshalJSONArray(tc.items)
			if err != nil {
				t.Fatalf("marshalJSONArray: %v", err)
			}
			if string(got) != tc.want {
				t.Errorf("got %s, want %s", got, tc.want)
			}
			if string(got) == "null" {
				t.Fatal("marshalJSONArray emitted null — zero-vs-missing distinction broken")
			}
		})
	}
}
