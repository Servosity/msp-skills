// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package mcp

import (
	"testing"

	"dnsfilter-pp-cli/internal/cli"
	"dnsfilter-pp-cli/internal/mcp/cobratree"
	"github.com/mark3labs/mcp-go/server"
)

// TestImportIsNotAnMCPTool guards handfixes.json: mcp-import-hidden. import
// reads a local JSONL file and POSTs one create per line, so an MCP caller
// must not see it as a tool.
func TestImportIsNotAnMCPTool(t *testing.T) {
	s := server.NewMCPServer("test", "0.0.0")
	cobratree.RegisterAll(s, cli.RootCmd(), func() (string, error) { return "missing-binary", nil })
	for name := range s.ListTools() {
		if name == "import" {
			t.Fatalf("import is exposed as MCP tool %q", name)
		}
	}
	if len(s.ListTools()) == 0 {
		t.Fatal("no tools registered; the walk did not run")
	}
}
