// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"testing"
)

// Hudu returns an asset's values under `fields` as {label,value} objects;
// `custom_fields` is the write shape. Every audit reads both.
func TestAssetFieldLabelsReadsHuduFieldsShape(t *testing.T) {
	var asset map[string]any
	if err := json.Unmarshal([]byte(`{"id":1,"asset_layout_id":3,"fields":[
		{"id":10,"label":"Hostname","value":"dc01","position":1},
		{"id":11,"label":"IP Address","value":"10.0.0.5","position":2},
		{"id":12,"label":"Role","value":"","position":3}]}`), &asset); err != nil {
		t.Fatal(err)
	}
	got := assetFieldLabels(asset)
	if !got["hostname"] || !got["ip address"] {
		t.Fatalf("filled labels from fields = %v, want hostname and ip address filled", got)
	}
	if filled, present := got["role"]; !present || filled {
		t.Fatalf("role = (filled %v, present %v), want present and empty", filled, present)
	}
}

func TestAssetFieldLabelsUnionsBothShapes(t *testing.T) {
	asset := map[string]any{
		"fields":        []any{map[string]any{"label": "Hostname", "value": ""}},
		"custom_fields": []any{map[string]any{"hostname": "dc01", "serial": "X1"}},
	}
	got := assetFieldLabels(asset)
	if !got["hostname"] || !got["serial"] {
		t.Fatalf("union = %v, want hostname (filled in custom_fields) and serial", got)
	}
}
