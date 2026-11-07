// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (handfixes.json mcp-search-bulk-seat-schema).

package cli

import (
	"encoding/json"
	"testing"
)

// Every body the published params_schema allows is a body the guard sends, and
// a body outside it is refused, so an agent reading the schema never builds a
// request the guard rejects.
func TestBulkSeatChangeParamsSchemaMatchesGuard(t *testing.T) {
	props := BulkSeatChangeParamsSchema()["properties"].(map[string]any)
	seats := props["seat_type"].(map[string]any)["enum"].([]string)
	actions := props["action_type"].(map[string]any)["enum"].([]string)
	if len(seats) == 0 || len(actions) == 0 {
		t.Fatal("schema enums are empty")
	}
	for _, s := range seats {
		for _, a := range actions {
			body, _ := json.Marshal(map[string]any{"seat_type": s, "action_type": a, "ids": []string{"remote-1"}})
			if _, err := canonicalBulkSeatBody(body); err != nil {
				t.Errorf("schema allows seat_type=%s action_type=%s but the guard refuses it: %v", s, a, err)
			}
		}
	}
	for _, bad := range []map[string]any{
		{"seat_type": "mcp-gate-probe", "action_type": actions[0], "ids": []string{"remote-1"}},
		{"seat_type": seats[0], "action_type": actions[0], "ids": []string{}},
	} {
		body, _ := json.Marshal(bad)
		if _, err := canonicalBulkSeatBody(body); err == nil {
			t.Errorf("guard accepted a body outside the schema: %s", body)
		}
	}
}
