// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package mcp

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/server"

	"threatlocker-pp-cli/internal/cli"
	"threatlocker-pp-cli/internal/store"
)

const tlLegacyMCPSentinel = "test-secret-legacy-mcp-key"

// seedV0312StoreAtMCPPath writes, at the path the MCP server reads, a store
// shaped like one the released v0.3.12 binary (schema version 9) leaves after
// `organizations auth-key` and `computers install-info`.
func seedV0312StoreAtMCPPath(t *testing.T) {
	t.Helper()
	resetMCPPathEnv(t)
	path, err := cli.MCPStorePath()
	if err != nil {
		t.Fatal(err)
	}
	s, err := store.OpenWithContext(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	db := s.DB()
	for _, c := range []struct{ table, column string }{
		{"organizations", "auth_key"}, {"computers", "install_key"}, {"computers", "download_url"},
	} {
		var n int
		if err := db.QueryRow(`SELECT count(*) FROM pragma_table_info(?) WHERE name = ?`, c.table, c.column).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n == 0 {
			if _, err := db.Exec(fmt.Sprintf(`ALTER TABLE %q ADD COLUMN %q TEXT`, c.table, c.column)); err != nil {
				t.Fatal(err)
			}
		}
	}
	orgJSON := `{"organizationId":"org-1","name":"Tenant 1","authKey":"` + tlLegacyMCPSentinel + `"}`
	compJSON := `{"computerId":"c1","computerName":"PC-1","installKey":"` + tlLegacyMCPSentinel + `"}`
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO organizations (id, data, organization_id, name, auth_key) VALUES ('org-1', ?, 'org-1', 'Tenant 1', ?)`, []any{orgJSON, tlLegacyMCPSentinel}},
		{`INSERT INTO computers (id, data, computer_id, computer_name, organization_id, install_key) VALUES ('c1', ?, 'c1', 'PC-1', 'org-1', ?)`, []any{compJSON, tlLegacyMCPSentinel}},
		{`INSERT INTO resources (id, resource_type, data) VALUES ('org-1', 'organizations', ?)`, []any{orgJSON}},
		{`INSERT INTO resources (id, resource_type, data) VALUES ('c1', 'computers', ?)`, []any{compJSON}},
		{`PRAGMA user_version = 9`, nil},
	} {
		if _, err := db.Exec(q.sql, q.args...); err != nil {
			t.Fatalf("seed %q: %v", q.sql, err)
		}
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
}

// Codex r2 / adversarial r2 P1: after an upgrade from v0.3.12, the MCP sql and
// search tools (read-only opens) returned the stored installation key until
// some CLI command happened to open the store for writing. Every read-only
// MCP store tool must answer from an upgraded store, first call included.
func TestMCPStoreToolsNeverReturnAKeyAV0312StoreHeld(t *testing.T) {
	for _, tc := range []struct {
		name    string
		handler server.ToolHandlerFunc
		args    map[string]any
	}{
		{"sql organizations", handleSQL, map[string]any{"query": "SELECT * FROM organizations"}},
		{"sql computers", handleSQL, map[string]any{"query": "SELECT * FROM computers"}},
		{"sql resources", handleSQL, map[string]any{"query": "SELECT data FROM resources"}},
		{"search", handleSearch, map[string]any{"query": tlLegacyMCPSentinel}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			seedV0312StoreAtMCPPath(t)
			got := tlResultText(tlCall(t, tc.handler, tc.args))
			if strings.Contains(got, tlLegacyMCPSentinel) {
				t.Fatalf("MCP %s returned the installation key a v0.3.12 store held:\n%s", tc.name, got)
			}
			if strings.HasPrefix(tc.name, "sql") && !strings.Contains(got, "org-1") && !strings.Contains(got, "c1") {
				t.Fatalf("MCP %s answered nothing, so the check above is vacuous:\n%s", tc.name, got)
			}
		})
	}
}
