// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package mcp

import "testing"

// TestMCPPathValueEscapesOpaqueID proves mcpPathValue percent-encodes an
// opaque single-segment note id, including the slash that
// cliutil.EscapePathParam would have deliberately preserved.
func TestMCPPathValueEscapesOpaqueID(t *testing.T) {
	cases := map[string]string{
		"not_06Yq6JtogRihEr": "not_06Yq6JtogRihEr",
		"a b":                "a%20b",
		"a/b":                "a%2Fb",
		"a?b":                "a%3Fb",
		"..":                 "..",
	}
	for in, want := range cases {
		if got := mcpPathValue(in); got != want {
			t.Errorf("mcpPathValue(%q) = %q, want %q", in, got, want)
		}
	}
}
