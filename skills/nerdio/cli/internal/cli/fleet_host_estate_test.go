// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored tests for the fleet host-estate novel feature.

package cli

import (
	"encoding/json"
	"testing"
)

func TestExtractStringAnyCaseInsensitive(t *testing.T) {
	obj := map[string]json.RawMessage{
		"PowerState": json.RawMessage(`"VM running"`),
		"empty":      json.RawMessage(`""`),
	}
	if v, ok := extractStringAny(obj, "powerState"); !ok || v != "VM running" {
		t.Errorf("extractStringAny case-insensitive = (%q,%v)", v, ok)
	}
	if _, ok := extractStringAny(obj, "empty"); ok {
		t.Error("empty string must not count as extracted")
	}
	if _, ok := extractStringAny(obj, "missing"); ok {
		t.Error("missing key must not extract")
	}
}

func TestExtractBoolAny(t *testing.T) {
	obj := map[string]json.RawMessage{
		"enableAutoscale": json.RawMessage(`false`),
		"notBool":         json.RawMessage(`"yes"`),
	}
	if v, ok := extractBoolAny(obj, "enableAutoscale"); !ok || v != false {
		t.Errorf("extractBoolAny = (%v,%v), want (false,true)", v, ok)
	}
	if _, ok := extractBoolAny(obj, "notBool"); ok {
		t.Error("string value must not extract as bool")
	}
}

func TestCapFleetAccounts(t *testing.T) {
	accounts := []fleetAccount{{ID: 1}, {ID: 2}, {ID: 3}}
	capped, truncated := capFleetAccounts(accounts, 2)
	if len(capped) != 2 || !truncated {
		t.Errorf("capFleetAccounts(3, max 2) = len %d truncated %v", len(capped), truncated)
	}
	capped, truncated = capFleetAccounts(accounts, 50)
	if len(capped) != 3 || truncated {
		t.Errorf("capFleetAccounts(3, max 50) = len %d truncated %v", len(capped), truncated)
	}
}
