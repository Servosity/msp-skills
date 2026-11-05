// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package mcp

import "testing"

// TestNoEndpointToolTakesAFile: the console logo uploads are CLI-only, so an MCP
// tool argument can never carry a file. A reprint re-emits them and fails here
// (handfixes.json umbrella-mcp-takes-no-file).
func TestNoEndpointToolTakesAFile(t *testing.T) {
	for _, e := range codeOrchEndpoints {
		if e.Multipart || len(e.FileParams) > 0 {
			t.Errorf("%s takes a file over MCP (multipart=%v, file params %v)", e.ID, e.Multipart, e.FileParams)
		}
	}
}
