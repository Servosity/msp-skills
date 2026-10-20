// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored (handfixes.json: mcp-read-only-drops-endpoint-writes).

package mcp

import (
	"testing"

	"github.com/mark3labs/mcp-go/server"
)

// endpointWriteTools are generated endpoint tools that change ION state.
var endpointWriteTools = []string{
	"customer_orders_cancel-order", "customer_orders_create-order", "carts_checkout",
	"carts_create", "carts_update", "carts_delete", "cart_items_create", "cart_items_update",
	"cart_items_delete", "customers_create", "customers_update", "products_enable",
	"products_disable", "programs_batch-update-product", "cloud_providers_link-customer",
	"cloud_providers_provision-customer", "reports_save", "reports_delete",
}

// TestReadOnlyEnvDropsEndpointWrites fails if TD_SYNNEX_MCP_READ_ONLY=1 still
// offers a generated endpoint tool that writes to StreamOne ION.
func TestReadOnlyEnvDropsEndpointWrites(t *testing.T) {
	t.Setenv("TD_SYNNEX_MCP_READ_ONLY", "1")
	s := server.NewMCPServer("read-only", "test")
	RegisterTools(s)
	tools := s.ListTools()
	for _, name := range append(endpointWriteTools, "subscriptions_set", "teach", "teach_playbook", "learnings_confirm") {
		if _, ok := tools[name]; ok {
			t.Errorf("read-only mode still registers write tool %s", name)
		}
	}
	for name, st := range tools {
		if hint := st.Tool.Annotations.ReadOnlyHint; hint == nil || !*hint {
			t.Errorf("read-only mode offers %s without readOnlyHint", name)
		}
	}
	for _, name := range []string{"subscriptions_list", "customers_list", "orders_list-account", "reports_get-data", "margin"} {
		if _, ok := tools[name]; !ok {
			t.Errorf("read-only mode dropped read tool %s", name)
		}
	}
}

// TestFullModeKeepsEndpointWrites pins the other direction: without the
// switch every write tool is still offered.
func TestFullModeKeepsEndpointWrites(t *testing.T) {
	t.Setenv("TD_SYNNEX_MCP_READ_ONLY", "")
	s := server.NewMCPServer("full", "test")
	RegisterTools(s)
	tools := s.ListTools()
	for _, name := range append(endpointWriteTools, "subscriptions_set") {
		if _, ok := tools[name]; !ok {
			t.Errorf("full mode is missing %s", name)
		}
	}
}

// TestOrderToolsAreDestructive fails if a tool that places, cancels or changes
// orders is offered without destructiveHint=true.
func TestOrderToolsAreDestructive(t *testing.T) {
	t.Setenv("TD_SYNNEX_MCP_READ_ONLY", "")
	s := server.NewMCPServer("full", "test")
	RegisterTools(s)
	tools := s.ListTools()
	for _, name := range destructiveEndpointTools {
		st, ok := tools[name]
		if !ok {
			t.Errorf("missing %s", name)
			continue
		}
		if h := st.Tool.Annotations.DestructiveHint; h == nil || !*h {
			t.Errorf("%s is not marked destructive", name)
		}
	}
}
