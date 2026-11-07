// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// microsoft-graph handfix (list-reads-every-page), class
// incomplete-read-as-complete, MCP twin: every endpoint list tool reports the
// whole collection. numberAssignments pages with $skip (default page 500, no
// nextLink); every other collection with @odata.nextLink.
func TestMCPListToolsReadEveryPage(t *testing.T) {
	resetMCPPathEnv(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		q := r.URL.Query()
		if strings.HasSuffix(r.URL.Path, "/numberAssignments") {
			top, skip := 500, 0
			if v := q.Get("$top"); v != "" {
				top, _ = strconv.Atoi(v)
			}
			top = min(top, 500) // the server caps every page at its default
			skip, _ = strconv.Atoi(q.Get("$skip"))
			page := []map[string]any{}
			for i := skip; i < 600 && i < skip+top; i++ {
				status := "userAssigned"
				if i >= 500 {
					status = "unassigned"
				}
				page = append(page, map[string]any{"id": fmt.Sprintf("num-%d", i), "assignmentStatus": status})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"value": page})
			return
		}
		if q.Get("$skiptoken") == "page2-7d1c9e" {
			_, _ = w.Write([]byte(`{"value":[{"id":"second-page-item"}]}`))
			return
		}
		next := "http://" + r.Host + r.URL.Path + "?$skiptoken=page2-7d1c9e"
		_, _ = fmt.Fprintf(w, `{"@odata.nextLink":%q,"value":[{"id":"first-page-item"}]}`, next)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("MICROSOFT_GRAPH_BASE_URL", srv.URL+"/v1.0")
	t.Setenv("MICROSOFT_GRAPH_TOKEN", "mcpListPagesToken-91b3")

	s := server.NewMCPServer("microsoft-graph", "test")
	RegisterTools(s)
	call := func(name string, args map[string]any) string {
		tool := s.GetTool(name)
		if tool == nil {
			t.Fatalf("tool %s is not registered", name)
		}
		var req mcplib.CallToolRequest
		req.Params.Name = name
		req.Params.Arguments = args
		res, err := tool.Handler(context.Background(), req)
		if err != nil || res == nil || len(res.Content) == 0 {
			t.Fatalf("%s: %v %v", name, err, res)
		}
		return res.Content[0].(mcplib.TextContent).Text
	}

	out := call("phone-numbers_list", map[string]any{})
	if n, u := strings.Count(out, `"num-`), strings.Count(out, `"unassigned"`); n != 600 || u != 100 {
		t.Errorf("phone-numbers_list returned %d numbers (%d unassigned), want 600 (100 unassigned):\n%.300s", n, u, out)
	}
	for _, name := range []string{"users_list", "groups_list", "devices_list", "directory-roles_list", "managed-devices_list",
		"security_alerts", "security_incidents", "licenses_skus", "teams-users_list"} {
		if s.GetTool(name) == nil {
			t.Errorf("tool %s is not registered", name)
			continue
		}
		if out := call(name, map[string]any{}); !strings.Contains(out, "second-page-item") {
			t.Errorf("%s returned only the first page:\n%.600s", name, out)
		}
	}
	for name, args := range map[string]map[string]any{
		"users_messages": {"id": "u1"}, "groups_members": {"id": "g1"}, "groups_owners": {"id": "g1"},
		"directory-roles_members": {"id": "r1"}, "users_licenses": {"id": "u1"},
	} {
		if s.GetTool(name) == nil {
			t.Errorf("tool %s is not registered", name)
			continue
		}
		if out := call(name, args); !strings.Contains(out, "second-page-item") {
			t.Errorf("%s returned only the first page:\n%.600s", name, out)
		}
	}
}
