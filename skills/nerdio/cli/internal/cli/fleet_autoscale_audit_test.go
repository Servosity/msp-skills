// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored tests for the fleet autoscale-audit novel feature.

package cli

import (
	"encoding/json"
	"testing"
)

func TestJSONEqual(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{`true`, `true`, true},
		{`true`, `false`, false},
		{`{"a":1,"b":2}`, `{ "b": 2, "a": 1 }`, true},
		{`[1,2]`, `[1,2]`, true},
		{`[1,2]`, `[2,1]`, false},
		{`"x"`, `"x"`, true},
		{`1`, `"1"`, false},
	}
	for _, tc := range cases {
		if got := jsonEqual(json.RawMessage(tc.a), json.RawMessage(tc.b)); got != tc.want {
			t.Errorf("jsonEqual(%s, %s) = %v, want %v", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestPoolIdentity(t *testing.T) {
	full := map[string]json.RawMessage{
		"subscriptionId": json.RawMessage(`"sub-1"`),
		"resourceGroup":  json.RawMessage(`"rg-1"`),
		"name":           json.RawMessage(`"pool-1"`),
	}
	sub, rg, name, ok := poolIdentity(full)
	if !ok || sub != "sub-1" || rg != "rg-1" || name != "pool-1" {
		t.Errorf("poolIdentity(full) = (%q,%q,%q,%v)", sub, rg, name, ok)
	}
	variant := map[string]json.RawMessage{
		"subscription":      json.RawMessage(`"sub-2"`),
		"resourceGroupName": json.RawMessage(`"rg-2"`),
		"hostPoolName":      json.RawMessage(`"pool-2"`),
	}
	sub, rg, name, ok = poolIdentity(variant)
	if !ok || sub != "sub-2" || rg != "rg-2" || name != "pool-2" {
		t.Errorf("poolIdentity(variant) = (%q,%q,%q,%v)", sub, rg, name, ok)
	}
	partial := map[string]json.RawMessage{"name": json.RawMessage(`"pool-3"`)}
	if _, _, _, ok := poolIdentity(partial); ok {
		t.Error("poolIdentity(partial) should not resolve without subscription + resource group")
	}
}

func TestSortedKeysDeterministic(t *testing.T) {
	m := map[string]json.RawMessage{"c": nil, "a": nil, "b": nil}
	got := sortedKeys(m)
	want := []string{"a", "b", "c"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("sortedKeys = %v, want %v", got, want)
		}
	}
}
