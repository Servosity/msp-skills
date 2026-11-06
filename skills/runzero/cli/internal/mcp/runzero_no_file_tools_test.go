// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
package mcp

import "testing"

// No MCP tool takes a file: the multipart asset import stays CLI-only, so an
// MCP argument can never name or carry a host file.
func TestNoEndpointToolTakesAFile(t *testing.T) {
	for _, ep := range codeOrchEndpoints {
		if ep.Multipart || len(ep.FileParams) > 0 {
			t.Errorf("%s (%s %s) takes a file over MCP", ep.ID, ep.Method, ep.Path)
		}
	}
}
