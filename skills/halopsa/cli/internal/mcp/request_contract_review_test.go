// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
package mcp

import "testing"

func TestRequestContractGETDoesNotAdvertiseUnsentBody(t *testing.T) {
	metadata := requestContractSearch(t, "take control", "take-control.list")
	schema := requestContractSchema(t, metadata)
	props := schema["properties"].(map[string]any)
	for name, raw := range props {
		if property, ok := raw.(map[string]any); ok && property["x-location"] == "body" {
			t.Errorf("GET advertises unsent body input %s: %#v", name, property)
		}
	}
	required, _ := schema["required"].([]any)
	for _, raw := range required {
		if raw == "body" {
			t.Error("GET requires a body that execute cannot send")
		}
	}
}
