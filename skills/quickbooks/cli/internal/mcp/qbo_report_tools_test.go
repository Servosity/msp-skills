// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package mcp

import (
	"testing"

	"github.com/mark3labs/mcp-go/server"
)

// The three financial statements are read-only MCP tools, with --columns and
// --raw exposed as parameters, and stay listed when QUICKBOOKS_MCP_READ_ONLY=1
// hides every tool that can change anything.
func TestMCPReportToolsAreReadOnly(t *testing.T) {
	for _, readOnlyEnv := range []string{"", "1"} {
		t.Run("QUICKBOOKS_MCP_READ_ONLY="+readOnlyEnv, func(t *testing.T) {
			t.Setenv("QUICKBOOKS_MCP_READ_ONLY", readOnlyEnv)
			s := server.NewMCPServer("quickbooks", "test")
			RegisterTools(s)
			tools := s.ListTools()
			for _, name := range []string{"report_pnl", "report_balance_sheet", "report_cash_flow"} {
				entry, ok := tools[name]
				if !ok {
					t.Fatalf("tool %s is not registered", name)
				}
				hint := entry.Tool.Annotations.ReadOnlyHint
				if hint == nil || !*hint {
					t.Errorf("tool %s is not marked read-only", name)
				}
				if d := entry.Tool.Annotations.DestructiveHint; d == nil || *d {
					t.Errorf("tool %s is not marked non-destructive", name)
				}
				for _, param := range []string{"columns", "raw"} {
					if _, ok := entry.Tool.InputSchema.Properties[param]; !ok {
						t.Errorf("tool %s has no %q parameter", name, param)
					}
				}
			}
			cf := tools["report_cash_flow"].Tool.InputSchema.Properties
			for _, param := range []string{"start", "end", "month", "cash"} {
				if _, ok := cf[param]; !ok {
					t.Errorf("report_cash_flow has no %q parameter", param)
				}
			}
		})
	}
}
