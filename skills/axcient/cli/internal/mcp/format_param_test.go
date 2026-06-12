// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package mcp

import (
	"encoding/json"
	"testing"
)

// TestFormatMCPParamValue_NoScientificNotation guards the fix for the MCP
// numeric-ID defect: JSON numbers decode to float64, and fmt's %v renders
// 7-digit Axcient device_id / appliance_id path params in scientific notation
// (1234567 -> "1.234567e+06"), which the API then 404s on. Path substitution
// must produce a plain integer string.
func TestFormatMCPParamValue_NoScientificNotation(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want string
	}{
		{"7-digit device id (float64 from JSON)", float64(1234567), "1234567"},
		{"appliance id with trailing zeros", float64(1230000), "1230000"},
		{"13-digit id", float64(1234567890123), "1234567890123"},
		{"json.Number id", json.Number("4977214"), "4977214"},
		{"string id passthrough", "abc-123", "abc-123"},
		{"bool flag", true, "true"},
		{"small client id", float64(12345), "12345"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := formatMCPParamValue(tc.in)
			if got != tc.want {
				t.Fatalf("formatMCPParamValue(%v) = %q, want %q (scientific notation here would 404 the API)", tc.in, got, tc.want)
			}
		})
	}
}
