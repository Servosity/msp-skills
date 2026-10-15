// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored (handfixes.json: mcp-read-only-drops-endpoint-writes).

package mcp

import (
	"testing"

	"github.com/mark3labs/mcp-go/server"
)

// TestReadOnlyEnvDropsEndpointWrites fails if QUOTEWERKS_MCP_READ_ONLY=1 still
// offers a generated endpoint tool that writes to QuoteWerks Web.
func TestReadOnlyEnvDropsEndpointWrites(t *testing.T) {
	t.Setenv("QUOTEWERKS_MCP_READ_ONLY", "1")
	s := server.NewMCPServer("read-only", "test")
	RegisterTools(s)
	tools := s.ListTools()
	// The typed writes (docs_create, items_add, items_reprice, requote) stay:
	// they preview, and the Cobra-tree gate refuses --confirm in this mode.
	for _, name := range []string{"agent_ask", "agent_report-draft", "reports_run"} {
		if _, ok := tools[name]; ok {
			t.Errorf("read-only mode still registers POST endpoint tool %s", name)
		}
	}
	for _, name := range []string{"tables_list", "tables_rows", "tables_query", "docs_search", "margin"} {
		if _, ok := tools[name]; !ok {
			t.Errorf("read-only mode dropped read tool %s", name)
		}
	}
}

// TestRawTableWritesAreCLIOnly: the raw table write tools carry no body
// parameter, so they are never offered over MCP, in either mode.
func TestRawTableWritesAreCLIOnly(t *testing.T) {
	t.Setenv("QUOTEWERKS_MCP_READ_ONLY", "")
	s := server.NewMCPServer("full", "test")
	RegisterTools(s)
	tools := s.ListTools()
	for _, name := range []string{"tables_insert", "tables_update", "tables_delete"} {
		if _, ok := tools[name]; ok {
			t.Errorf("full mode still offers %s", name)
		}
	}
	for _, name := range []string{"docs_create", "items_add", "items_reprice", "requote", "tables_rows", "agent_ask", "reports_run"} {
		if _, ok := tools[name]; !ok {
			t.Errorf("full mode is missing %s", name)
		}
	}
}
