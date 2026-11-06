// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored (handfixes.json: mcp-order-tools-destructive-hint). The read-only drop is native in 4.35.1 (tools.go).

package mcp

import (
	"github.com/mark3labs/mcp-go/server"
)

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
