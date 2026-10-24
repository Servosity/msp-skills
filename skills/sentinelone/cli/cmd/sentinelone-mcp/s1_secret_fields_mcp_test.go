// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): tests for the MCP tool-result middleware that
// withholds credential fields (s1_secret_fields_mcp.go).

package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"sentinelone-pp-cli/internal/cli"
	"sentinelone-pp-cli/internal/cliutil/testenv"
	mcptools "sentinelone-pp-cli/internal/mcp"
	"sentinelone-pp-cli/internal/store"
)

func secretToolText(t *testing.T, res *mcplib.CallToolResult) string {
	t.Helper()
	text := ""
	for _, c := range res.Content {
		if tc, ok := c.(mcplib.TextContent); ok {
			text += tc.Text
		}
	}
	return text
}

func callWithheld(t *testing.T, tool string, args map[string]any, text string) string {
	t.Helper()
	h := withholdToolSecrets(func(context.Context, mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		return mcplib.NewToolResultText(text), nil
	})
	req := mcplib.CallToolRequest{}
	req.Params.Name = tool
	req.Params.Arguments = args
	res, err := h(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	return secretToolText(t, res)
}

// TestMCPToolResultsWithholdSecrets: a credential field in any tool result
// (a live site read's registrationToken, a sql row's data column, a bounded
// preview cut mid-value, tail's NDJSON, a name/value config pair) is withheld
// from the agent; only sentinelone_execute with confirm: true returns it, and
// a result with nothing to withhold is returned byte for byte.
func TestMCPToolResultsWithholdSecrets(t *testing.T) {
	testenv.Isolate(t) // the sql case scrubs the (sandboxed) local store first
	const tok = "mcpsiteregtokensentinel"
	sitesJSON := `{"data":{"sites":[{"id":"s1","name":"Site One","registrationToken":"` + tok + `"}]},"pagination":{"nextCursor":"abc"}}`
	sqlRow, _ := json.Marshal(map[string]any{"rows": []any{map[string]any{"data": `{"id":"11","passphrase":"MCP PASS PHRASE SENTINEL"}`}}})
	preview, _ := json.Marshal(map[string]any{"preview": `{"data":[{"id":"s1","registrationToken":"` + tok[:12], "truncated": true})
	ndjson := `{"id":"g1","registrationToken":"` + tok + `"}` + "\n" + `{"id":"g2","registrationToken":"` + tok + `"}` + "\n"
	pair := `{"data":[{"config":[{"name":"apiKey","value":"mcpmarketplacekeysentinel"},{"name":"region","value":"us-east"}]}]}`

	for name, tc := range map[string]struct {
		tool, text string
		gone       []string
		kept       []string
	}{
		"live read":     {"sentinelone_read", sitesJSON, []string{tok}, []string{"Site One", "nextCursor", "abc"}},
		"sql data col":  {"sql", string(sqlRow), []string{"MCP PASS PHRASE SENTINEL"}, []string{`\"id\":\"11\"`}},
		"preview cut":   {"sentinelone_read", string(preview), []string{tok[:12]}, []string{"truncated"}},
		"tail ndjson":   {"tail", ndjson, []string{tok}, []string{"g1", "g2"}},
		"name/value":    {"sentinelone_read", pair, []string{"mcpmarketplacekeysentinel"}, []string{"us-east", "apiKey"}},
		"unconfirmed x": {"sentinelone_execute", sitesJSON, []string{tok}, []string{"Site One"}},
	} {
		t.Run(name, func(t *testing.T) {
			got := callWithheld(t, tc.tool, map[string]any{"confirm": "true"}, tc.text)
			for _, g := range tc.gone {
				if strings.Contains(got, g) {
					t.Errorf("%s reached the agent: %s", g, got)
				}
			}
			for _, k := range tc.kept {
				if !strings.Contains(got, k) {
					t.Errorf("ordinary %q lost: %s", k, got)
				}
			}
		})
	}

	if got := callWithheld(t, "sentinelone_execute", map[string]any{"confirm": true}, sitesJSON); got != sitesJSON {
		t.Fatalf("a confirmed execute was altered: %s", got)
	}
	clean := `{"data":[{"id":"a1","name":"tokenless", "summary":"Get the uninstall password metadata"}]}`
	if got := callWithheld(t, "sentinelone_search", nil, clean); got != clean {
		t.Fatalf("a clean result was altered: %s", got)
	}
}

// TestMCPStoreToolsScrubAnOldStoreFirst: a store an older build wrote still
// holds agent passphrases; the first sql or search call scrubs it before it
// reads, so even a column alias the field rule cannot see returns nothing.
func TestMCPStoreToolsScrubAnOldStoreFirst(t *testing.T) {
	testenv.Isolate(t)
	t.Setenv("SENTINELONE_BASE_URL", "http://127.0.0.1:1")
	t.Setenv("SENTINELONE_API_TOKEN", "mcp-scrub-test-token")
	path, err := cli.MCPStorePath()
	if err != nil {
		t.Fatal(err)
	}
	s, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB().Exec(`INSERT INTO resources (id, resource_type, data) VALUES ('11', 'agents-passphrases', '{"id":"11","passphrase":"OLD STORE ALIAS SENTINEL"}')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB().Exec(`DROP TABLE IF EXISTS secret_scrub_state`); err != nil {
		t.Fatal(err)
	}
	_ = s.Close()

	srv := server.NewMCPServer("t", "0")
	mcptools.RegisterTools(srv)
	sqlTool := srv.GetTool("sql")
	if sqlTool == nil {
		t.Fatal("sql tool not registered")
	}
	req := mcplib.CallToolRequest{}
	req.Params.Name = "sql"
	req.Params.Arguments = map[string]any{"query": "SELECT json_extract(data,'$.passphrase') AS t FROM resources"}
	res, err := withholdToolSecrets(sqlTool.Handler)(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if text := secretToolText(t, res); strings.Contains(text, "OLD STORE ALIAS SENTINEL") {
		t.Fatalf("sql returned an old store's passphrase: %s", text)
	}
}
