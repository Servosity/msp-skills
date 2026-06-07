// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored tests for the usages drift novel feature.

package cli

import (
	"encoding/json"
	"testing"
)

func TestDecodeObjects(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want int
	}{
		{"bare array", `[{"a":1},{"a":2}]`, 2},
		{"items envelope", `{"items":[{"a":1}]}`, 1},
		{"data envelope", `{"data":[{"a":1},{"a":2},{"a":3}]}`, 3},
		{"single object", `{"id":7}`, 1},
		{"null", `null`, 0},
		{"empty", ``, 0},
		{"empty array", `[]`, 0},
	}
	for _, tc := range cases {
		got := decodeObjects(json.RawMessage(tc.in))
		if len(got) != tc.want {
			t.Errorf("%s: decodeObjects = %d objects, want %d", tc.name, len(got), tc.want)
		}
	}
}

func TestExtractNumberAnyStringEncoded(t *testing.T) {
	obj := map[string]json.RawMessage{
		"total":  json.RawMessage(`"19.91"`),
		"count":  json.RawMessage(`42`),
		"broken": json.RawMessage(`"n/a"`),
	}
	if v, ok := extractNumberAny(obj, "total"); !ok || v != 19.91 {
		t.Errorf("string-encoded number = (%v,%v), want (19.91,true)", v, ok)
	}
	if v, ok := extractNumberAny(obj, "count"); !ok || v != 42 {
		t.Errorf("json number = (%v,%v), want (42,true)", v, ok)
	}
	if _, ok := extractNumberAny(obj, "broken"); ok {
		t.Error("unparseable string must not extract")
	}
	if v, ok := extractNumberAny(obj, "missing", "total"); !ok || v != 19.91 {
		t.Errorf("fallback key order = (%v,%v), want (19.91,true)", v, ok)
	}
}

func TestFleetAccountsFromObjects(t *testing.T) {
	objs := []map[string]json.RawMessage{
		{"id": json.RawMessage(`101`), "name": json.RawMessage(`"Contoso"`)},
		{"accountId": json.RawMessage(`102`)},
		{"id": json.RawMessage(`101`), "name": json.RawMessage(`"Duplicate"`)},
		{"noId": json.RawMessage(`"x"`)},
	}
	got := fleetAccountsFromObjects(objs)
	if len(got) != 2 {
		t.Fatalf("fleetAccountsFromObjects = %d accounts, want 2 (dedup + skip no-id)", len(got))
	}
	if got[0].ID != 101 || got[0].Name != "Contoso" {
		t.Errorf("first account = %+v", got[0])
	}
	if got[1].ID != 102 || got[1].Name != "account-102" {
		t.Errorf("second account should fall back to synthesized name, got %+v", got[1])
	}
}
