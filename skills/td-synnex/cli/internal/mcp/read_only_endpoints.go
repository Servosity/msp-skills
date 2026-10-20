// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored (handfixes.json: mcp-read-only-drops-endpoint-writes).

package mcp

import (
	"sort"

	"github.com/mark3labs/mcp-go/server"
)

// dropNonReadOnlyTools removes every registered tool that is not annotated
// read-only. The generated endpoint-mirror tools (customer_orders_cancel-order,
// carts_checkout, customer_orders_create-order, ...) never consulted
// TD_SYNNEX_MCP_READ_ONLY; only the Cobra-tree mirror applied it, so the kill
// switch still cancelled orders and checked out carts. Run it after
// cobratree.RegisterAll, whose own filter keeps local-store writers (teach,
// learnings_confirm), so one rule covers every tool.
func dropNonReadOnlyTools(s *server.MCPServer) []string {
	var drop []string
	for name, st := range s.ListTools() {
		if hint := st.Tool.Annotations.ReadOnlyHint; hint == nil || !*hint {
			drop = append(drop, name)
		}
	}
	sort.Strings(drop)
	if len(drop) > 0 {
		s.DeleteTools(drop...)
	}
	return drop
}

// destructiveEndpointTools place billable orders, cancel them, or change what
// a customer can buy. The generated annotations marked them non-destructive,
// so hosts confirmed them no louder than a read.
var destructiveEndpointTools = []string{
	"customer_orders_cancel-order", "customer_orders_create-order", "carts_checkout",
	"products_disable", "products_enable", "cloud_providers_provision-customer",
	"cloud_providers_link-customer", "programs_batch-update-product",
}

// markDestructiveTools re-registers those tools with destructiveHint=true.
func markDestructiveTools(s *server.MCPServer) {
	tools := s.ListTools()
	for _, name := range destructiveEndpointTools {
		st, ok := tools[name]
		if !ok {
			continue
		}
		tool := st.Tool
		destructive := true
		tool.Annotations.DestructiveHint = &destructive
		s.AddTool(tool, st.Handler)
	}
}
