// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package mcp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"cloudally-pp-cli/internal/cliutil"
	"cloudally-pp-cli/internal/cliutil/testenv"
)

func registeredTools(t *testing.T) map[string]*server.ServerTool {
	t.Helper()
	s := server.NewMCPServer("test", "0.0.0")
	RegisterTools(s)
	return s.ListTools()
}

// The switch lists no tool that is not annotated read-only, and live reads
// stay available without cloudally_execute.
func TestReadOnlySwitchListsOnlyReadTools(t *testing.T) {
	t.Setenv(cliutil.MCPReadOnlyEnvVar, "")
	all := registeredTools(t)
	for _, name := range []string{"cloudally_execute", "cloudally_read", "teach", "sync"} {
		if all[name] == nil {
			t.Errorf("%s missing with the switch off", name)
		}
	}

	t.Setenv(cliutil.MCPReadOnlyEnvVar, "1")
	ro := registeredTools(t)
	for name, tool := range ro {
		if hint := tool.Tool.Annotations.ReadOnlyHint; hint == nil || !*hint {
			t.Errorf("%s is listed under the switch without readOnlyHint", name)
		}
	}
	for _, name := range []string{"cloudally_search", "cloudally_get", "cloudally_read", "search", "sql", "context"} {
		if ro[name] == nil {
			t.Errorf("read tool %s is hidden by the switch", name)
		}
	}
}

// cloudally_read reads, and refuses every write without reaching the API, with
// or without confirm.
func TestCodeOrchReadRefusesWrites(t *testing.T) {
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"u-1"}`))
	}))
	defer srv.Close()
	testenv.Isolate(t, cliutil.ConfigDir, cliutil.DataDir, cliutil.StateDir, cliutil.CacheDir)
	t.Setenv("CLOUDALLY_BASE_URL", srv.URL)

	var write, read *codeOrchEndpoint
	for i := range codeOrchEndpoints {
		ep := &codeOrchEndpoints[i]
		if ep.Method != "GET" && write == nil {
			write = ep
		}
		if ep.Method == "GET" && !strings.Contains(ep.Path, "{") && read == nil {
			read = ep
		}
	}
	if write == nil || read == nil {
		t.Fatal("no write or read endpoint in the table")
	}

	call := func(id string, confirm bool) *mcplib.CallToolResult {
		t.Helper()
		res, err := handleCodeOrchRead(context.Background(), mcplib.CallToolRequest{Params: mcplib.CallToolParams{
			Arguments: map[string]any{"endpoint_id": id, "confirm": confirm},
		}})
		if err != nil {
			t.Fatalf("%s: transport error: %v", id, err)
		}
		return res
	}
	for _, confirm := range []bool{false, true} {
		if res := call(write.ID, confirm); !res.IsError {
			t.Errorf("cloudally_read ran %s %s with confirm=%v", write.Method, write.ID, confirm)
		}
	}
	if requests != 0 {
		t.Fatalf("a refused write reached the API (%d requests)", requests)
	}
	if res := call(read.ID, false); res.IsError || requests != 1 {
		t.Errorf("cloudally_read %s: isError=%v requests=%d", read.ID, res.IsError, requests)
	}
}
