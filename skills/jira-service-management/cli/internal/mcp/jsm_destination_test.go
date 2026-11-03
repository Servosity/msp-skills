// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package mcp

import (
	"testing"

	"github.com/mark3labs/mcp-go/server"
)

// Handfix jsm-mcp-no-site-selection: the site (and so where the Basic
// credential goes) is the operator's choice, never a tool argument. sync's
// --path-context site=<other> re-targets the configured credential at another
// client's site, so no tool takes it.
func TestNoToolSelectsTheSite(t *testing.T) {
	s := server.NewMCPServer("site", "test")
	RegisterTools(s)
	for name, entry := range s.ListTools() {
		if _, ok := entry.Tool.InputSchema.Properties["path-context"]; ok {
			t.Errorf("%s takes path-context, which can send the credential to another site", name)
		}
	}
}
