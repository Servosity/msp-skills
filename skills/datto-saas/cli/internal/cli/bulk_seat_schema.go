// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (handfixes.json mcp-search-bulk-seat-schema). The MCP search
// result for seats.bulk-change carries this params_schema so an agent knows
// the body the transport guard accepts before it calls execute. Built from the
// guard's own value lists, so the schema and the guard cannot disagree.

package cli

// BulkSeatChangeParamsSchema describes the executor params for
// PUT /saas/{saasCustomerId}/{externalSubscriptionId}/bulkSeatChange.
func BulkSeatChangeParamsSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"saasCustomerId", "externalSubscriptionId", "seat_type", "action_type", "ids"},
		"properties": map[string]any{
			"saasCustomerId":         map[string]any{"type": "string", "x-location": "path"},
			"externalSubscriptionId": map[string]any{"type": "string", "x-location": "path"},
			"seat_type":              map[string]any{"type": "string", "x-location": "body", "enum": append([]string(nil), saasSeatTypes...)},
			"action_type":            map[string]any{"type": "string", "x-location": "body", "enum": append([]string(nil), bulkSeatActions...)},
			"ids": map[string]any{
				"type": "array", "x-location": "body", "minItems": 1, "maxItems": maxBulkSeatIDs,
				"items":       map[string]any{"type": "string", "pattern": `\S`},
				"description": "Seat remoteId values from seats list (a JSON array or one comma-separated string).",
			},
		},
	}
}
