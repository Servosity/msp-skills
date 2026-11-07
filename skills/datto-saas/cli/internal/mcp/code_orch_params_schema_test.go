// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (handfixes.json mcp-search-bulk-seat-schema).

package mcp

import "testing"

func TestCodeOrchSearchPublishesBulkSeatParamsSchema(t *testing.T) {
	ep := findCodeOrchEndpoint("seats.bulk-change")
	if ep == nil {
		t.Fatal("seats.bulk-change is not a code-orchestration endpoint")
	}
	schema, ok := codeOrchEndpointMetadata(ep)["params_schema"].(map[string]any)
	if !ok {
		t.Fatal("search metadata for seats.bulk-change has no params_schema")
	}
	req, _ := schema["required"].([]string)
	want := map[string]bool{"seat_type": true, "action_type": true, "ids": true}
	for _, r := range req {
		delete(want, r)
	}
	if len(want) != 0 {
		t.Fatalf("params_schema.required misses %v", want)
	}
}
