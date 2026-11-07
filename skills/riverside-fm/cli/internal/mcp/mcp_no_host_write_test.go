// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package mcp

import (
	"testing"

	"github.com/mark3labs/mcp-go/server"
)

// No MCP tool argument may make the server write a file on the host. grab and
// bulk export always write media to disk, so they are not tools at all; clips
// harvest and media refresh are read-only unless --download/--prefetch, so
// those flags (and --out) are not in their schemas.
func TestMCPToolsNeverWriteHostFiles(t *testing.T) {
	s := server.NewMCPServer("riverside-fm", "test")
	RegisterTools(s)
	tools := s.ListTools()
	for _, name := range []string{"grab", "bulk_export"} {
		if _, ok := tools[name]; ok {
			t.Errorf("tool %q writes files to the host and must not be exposed over MCP", name)
		}
	}
	for name, banned := range map[string][]string{
		"clips_harvest": {"download", "out"},
		"media_refresh": {"prefetch", "out"},
	} {
		tl, ok := tools[name]
		if !ok {
			t.Errorf("read-only tool %q is missing", name)
			continue
		}
		for _, p := range banned {
			if _, ok := tl.Tool.InputSchema.Properties[p]; ok {
				t.Errorf("tool %q exposes %q, which writes files to the host", name, p)
			}
		}
	}
}
