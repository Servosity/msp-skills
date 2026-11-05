// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	"superops-pp-cli/internal/store"
)

const mcpStampTable = "superops_store_tenant"

// seedMCPStore writes tenant rows into the store MCP reads. unproven drops the
// tenant stamp afterwards, the shape of a store written before the stamp.
func seedMCPStore(t *testing.T, unproven bool, ids ...string) string {
	t.Helper()
	path, err := mcpDBPath()
	if err != nil {
		t.Fatalf("mcpDBPath() error = %v", err)
	}
	db, err := store.Open(path)
	if err != nil {
		t.Fatalf("seed store: %v", err)
	}
	var rows []json.RawMessage
	for i, id := range ids {
		row, _ := json.Marshal(map[string]any{
			"ticketId": fmt.Sprintf("9200000000000%04d", i+1), "displayId": id, "subject": "seeded " + id,
			"updatedTime": "2026-01-01T00:00:00", "client": map[string]any{"name": "Acme"},
		})
		rows = append(rows, row)
	}
	if _, _, err := db.UpsertBatch("tickets", rows); err != nil {
		t.Fatalf("seed rows: %v", err)
	}
	if err := db.SaveSyncState("tickets", "", len(rows)); err != nil {
		t.Fatalf("seed checkpoint: %v", err)
	}
	if unproven {
		if _, err := db.DB().Exec(`DROP TABLE IF EXISTS ` + mcpStampTable); err != nil {
			t.Fatalf("drop stamp: %v", err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func assertMCPRefusal(t *testing.T, what string, result *mcplib.CallToolResult, leaked ...string) {
	t.Helper()
	text := mcpTextContent(t, result)
	if !result.IsError {
		t.Fatalf("%s was not refused:\n%s", what, text)
	}
	for _, l := range leaked {
		if strings.Contains(text, l) {
			t.Fatalf("%s returned another tenant's row %s:\n%s", what, l, text)
		}
	}
	if !strings.Contains(text, "tenant") || !strings.Contains(text, "sync") {
		t.Fatalf("%s refusal does not name the remedy:\n%s", what, text)
	}
}

func mcpSearchResult(t *testing.T, query string) *mcplib.CallToolResult {
	t.Helper()
	result, err := handleSearch(context.Background(), mcplib.CallToolRequest{Params: mcplib.CallToolParams{
		Arguments: map[string]any{"query": query},
	}})
	if err != nil || result == nil {
		t.Fatalf("handleSearch transport error: %v", err)
	}
	return result
}

// Member 2 (adversarial P2): superops-mcp with no SUPEROPS_API_TOKEN reads the
// unscoped data.db. One holding tenant A's rows, unstamped or stamped for A,
// is refused to anyone configured as another tenant or as no tenant.
func TestMCPSQLAndSearchRefuseAStoreOfAnotherTenant(t *testing.T) {
	for _, unproven := range []bool{true, false} {
		t.Run(fmt.Sprintf("unproven=%v", unproven), func(t *testing.T) {
			resetMCPPathEnv(t)
			t.Setenv("SUPEROPS_API_TOKEN", "")
			t.Setenv("SUPEROPS_SUBDOMAIN", "acme-a")
			path := seedMCPStore(t, unproven, "A-1", "A-2")
			if !strings.HasSuffix(path, "data.db") {
				t.Fatalf("with no credential MCP reads %s, want the unscoped data.db", path)
			}
			for _, subdomain := range []string{"acme-b", ""} {
				t.Setenv("SUPEROPS_SUBDOMAIN", subdomain)
				label := fmt.Sprintf("subdomain %q", subdomain)
				assertMCPRefusal(t, label+" sql", callMCPSQL(t, context.Background(), "SELECT resource_type, id, data FROM resources"), "A-1", "A-2", "92000000000000001")
				assertMCPRefusal(t, label+" search", mcpSearchResult(t, "seeded"), "A-1", "A-2")
			}
		})
	}
}

// Positive path: MCP sql and search reach a store stamped for their own tenant.
func TestMCPSQLAndSearchReachTheirOwnTenantsStore(t *testing.T) {
	for _, token := range []string{"", "sop-own-tenant-token"} {
		t.Run(fmt.Sprintf("token=%v", token != ""), func(t *testing.T) {
			resetMCPPathEnv(t)
			t.Setenv("SUPEROPS_API_TOKEN", token)
			t.Setenv("SUPEROPS_SUBDOMAIN", "acme-b")
			seedMCPStore(t, false, "B-1", "B-2")

			result := callMCPSQL(t, context.Background(), "SELECT id FROM resources WHERE resource_type = 'tickets'")
			text, envelope := decodeMCPSQLEnvelope(t, result)
			if result.IsError || envelope.Count != 2 {
				t.Fatalf("own-tenant sql: IsError %v count %d\n%s", result.IsError, envelope.Count, text)
			}
			search := mcpSearchResult(t, "seeded")
			if search.IsError || !strings.Contains(mcpTextContent(t, search), "B-1") {
				t.Fatalf("own-tenant search:\n%s", mcpTextContent(t, search))
			}
		})
	}
}
