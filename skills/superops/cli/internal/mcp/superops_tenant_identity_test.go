// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package mcp

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"superops-pp-cli/internal/cli"
	"superops-pp-cli/internal/config"
	"superops-pp-cli/internal/store"
)

const mcpURLPassword = "PwUserinfoSecret4471"

var mcpIdentityOpRe = regexp.MustCompile(`\b(get\w+List|getKbItems|getWorklogEntries)\s*\(`)

// serveTwoTickets answers every SuperOps list query on POST /msp with two
// tickets for getTicketList and nothing for the rest, on one page.
func serveTwoTickets(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Query string `json:"query"`
		}
		m := mcpIdentityOpRe.FindStringSubmatch(func() string { _ = json.Unmarshal(body, &req); return req.Query }())
		if r.Method != http.MethodPost || r.URL.Path != "/msp" || m == nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		var nodes []map[string]any
		if m[1] == "getTicketList" {
			for _, id := range []string{"7300000000000001", "7300000000000002"} {
				nodes = append(nodes, map[string]any{"ticketId": id, "displayId": "M-" + id[len(id)-1:], "subject": "mcp ticket", "status": "Open", "updatedTime": "2026-01-01T00:00:00"})
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{m[1]: map[string]any{
			"nodes": nodes, "listInfo": map[string]any{"hasMore": false, "page": 1, "pageSize": 50, "totalCount": len(nodes)},
		}}})
	}))
	t.Cleanup(srv.Close)
	return strings.TrimPrefix(srv.URL, "http://")
}

func runCLI(t *testing.T, args ...string) {
	t.Helper()
	cmd := cli.RootCmd()
	cmd.SetArgs(args)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("superops-cli %v: %v\n%s", args, err, out.String())
	}
}

// The adversarial round-2 path: a base URL with a password, a read-write open
// and a sync, then the MCP sql tool reads the tenant table, the checkpoints
// and every resource row. None of them returns the password.
func TestMCPSQLNeverReturnsTheBaseURLPassword(t *testing.T) {
	resetMCPPathEnv(t)
	host := serveTwoTickets(t)
	t.Setenv("SUPEROPS_BASE_URL", "http://svc:"+mcpURLPassword+"@"+host)
	t.Setenv("SUPEROPS_API_TOKEN", "sop-mcp-identity-token")
	t.Setenv("SUPEROPS_SUBDOMAIN", "acme-a")
	runCLI(t, "learnings", "list", "--json")
	runCLI(t, "sync", "--resources", "tickets")

	for _, query := range []string{
		"SELECT tenant FROM superops_store_tenant",
		"SELECT scope_key FROM sync_state",
		"SELECT resource_type, id, data FROM resources",
	} {
		result := callMCPSQL(t, context.Background(), query)
		text := mcpTextContent(t, result)
		if result.IsError {
			t.Fatalf("%s: refused its own tenant's store:\n%s", query, text)
		}
		if strings.Contains(strings.ToLower(text), strings.ToLower(mcpURLPassword)) {
			t.Fatalf("%s returned the base URL password:\n%s", query, text)
		}
	}
	text := mcpTextContent(t, callMCPSQL(t, context.Background(), "SELECT tenant FROM superops_store_tenant"))
	if !strings.Contains(text, "subdomain=acme-a base_url=http://"+host) {
		t.Fatalf("the tenant row is not the identity without the password:\n%s", text)
	}
}

// A store the earlier build stamped with the password is refused to the
// read-only MCP sql tool, with the remedy and without the record. After any
// read-write CLI command of its tenant it is readable again, without it.
func TestMCPSQLRefusesALegacyStampHoldingTheURLPassword(t *testing.T) {
	resetMCPPathEnv(t)
	raw := "http://svc:" + mcpURLPassword + "@127.0.0.1:9"
	t.Setenv("SUPEROPS_BASE_URL", raw)
	t.Setenv("SUPEROPS_API_TOKEN", "sop-mcp-identity-token")
	t.Setenv("SUPEROPS_SUBDOMAIN", "acme-a")
	legacy := "subdomain=acme-a base_url=" + strings.ToLower(raw)

	path, err := mcpDBPath()
	if err != nil {
		t.Fatal(err)
	}
	s, err := store.Open(path)
	if err != nil {
		t.Fatalf("plant open: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	// This build never writes a held value to a store, so the earlier
	// build's records are planted with the plain driver, as it wrote them.
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	legacyScope := "template:" + config.SuperOpsTenantScopeVar + "=" + legacy
	for _, stmt := range []string{
		`DROP TABLE IF EXISTS ` + mcpStampTable,
		`CREATE TABLE ` + mcpStampTable + ` (id INTEGER PRIMARY KEY CHECK (id = 1), tenant TEXT NOT NULL, stamped_at TEXT NOT NULL)`,
		`INSERT INTO ` + mcpStampTable + ` (id, tenant, stamped_at) VALUES (1, '` + legacy + `', '2026-10-26T00:00:00Z')`,
		`DELETE FROM sync_state WHERE resource_type = 'tickets'`,
		`INSERT INTO sync_state (resource_type, last_cursor, last_synced_at, total_count, last_attempt_complete, scope_key) VALUES ('tickets', '', '2026-10-26T00:00:00Z', 0, 1, '` + legacyScope + `')`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("plant %q: %v", stmt, err)
		}
	}
	_ = db.Close()

	for _, query := range []string{"SELECT tenant FROM superops_store_tenant", "SELECT scope_key FROM sync_state"} {
		result := callMCPSQL(t, context.Background(), query)
		text := mcpTextContent(t, result)
		if !result.IsError {
			t.Fatalf("%s: a legacy store holding the URL password was read:\n%s", query, text)
		}
		if strings.Contains(strings.ToLower(text), strings.ToLower(mcpURLPassword)) || strings.Contains(text, "base_url=") {
			t.Fatalf("%s: the refusal echoes the record:\n%s", query, text)
		}
		if !strings.Contains(text, "superops-cli") || !strings.Contains(text, "delete") {
			t.Fatalf("%s: the refusal does not name the remedy:\n%s", query, text)
		}
	}

	runCLI(t, "learnings", "list", "--json")
	for _, query := range []string{"SELECT tenant FROM superops_store_tenant", "SELECT scope_key FROM sync_state"} {
		result := callMCPSQL(t, context.Background(), query)
		text := mcpTextContent(t, result)
		if result.IsError || strings.Contains(strings.ToLower(text), strings.ToLower(mcpURLPassword)) {
			t.Fatalf("%s after a read-write command (IsError %v):\n%s", query, result.IsError, text)
		}
	}
}
