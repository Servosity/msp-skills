// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): a DSM stand-in for the press 4.33.5 template
// class tests recorded in skills/synology/handfixes.json.

package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"synology-pp-cli/internal/cliutil"
	"synology-pp-cli/internal/cliutil/testenv"
	"synology-pp-cli/internal/dsm/dsmtest"
)

// fakeDSMCall is one request the fake NAS received after login.
type fakeDSMCall struct {
	API, Method string
	Query       map[string]string
}

type fakeDSM struct {
	srv   *httptest.Server
	mu    sync.Mutex
	calls []fakeDSMCall
}

func (f *fakeDSM) Calls() []fakeDSMCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]fakeDSMCall(nil), f.calls...)
}

// newFakeDSM answers the DSM login (account msp-admin / s3cretpw), records
// every other call, and serves taskName in SYNO.ActiveBackup.Task list.
func newFakeDSM(t *testing.T, taskName string) *fakeDSM {
	t.Helper()
	f := &fakeDSM{}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if dsmtest.ServeAPIInfo(w, r) {
			return
		}
		_ = r.ParseForm()
		if r.Form.Get("api") == "SYNO.API.Auth" && r.Form.Get("method") == "login" {
			if r.Form.Get("account") != "msp-admin" || r.Form.Get("passwd") != "s3cretpw" {
				_, _ = w.Write([]byte(`{"success":false,"error":{"code":400}}`))
				return
			}
			_, _ = w.Write([]byte(`{"success":true,"data":{"sid":"sid-1","synotoken":"tok-1"}}`))
			return
		}
		q := map[string]string{}
		for k := range r.URL.Query() {
			q[k] = r.URL.Query().Get(k)
		}
		f.mu.Lock()
		f.calls = append(f.calls, fakeDSMCall{API: q["api"], Method: q["method"], Query: q})
		f.mu.Unlock()
		if q["_sid"] != "sid-1" {
			_, _ = w.Write([]byte(`{"success":false,"error":{"code":119}}`))
			return
		}
		switch q["api"] + "/" + q["method"] {
		case "SYNO.ActiveBackup.Task/list":
			b, _ := json.Marshal(map[string]any{"success": true, "data": map[string]any{"tasks": []any{map[string]any{"task_id": 5, "task_name": taskName}}}})
			_, _ = w.Write(b)
		default:
			_, _ = w.Write([]byte(`{"success":true,"data":{"ok":true}}`))
		}
	}))
	t.Cleanup(f.srv.Close)
	return f
}

// sweepMCPServer isolates the home, points the CLI config at fake, and
// registers every tool. readOnly sets SYNOLOGY_MCP_READ_ONLY.
func sweepMCPServer(t *testing.T, fake *fakeDSM, readOnly string) *server.MCPServer {
	t.Helper()
	testenv.Isolate(t, cliutil.ConfigDir, cliutil.DataDir, cliutil.StateDir, cliutil.CacheDir)
	t.Setenv("SYNOLOGY_NO_LEARN", "1")
	t.Setenv("SYNOLOGY_ACCOUNT", "msp-admin")
	t.Setenv("SYNOLOGY_PASSWORD", "s3cretpw")
	t.Setenv("SYNOLOGY_MCP_READ_ONLY", readOnly)
	if fake != nil {
		t.Setenv("SYNOLOGY_BASE_URL", fake.srv.URL)
	}
	s := server.NewMCPServer("synology", "test")
	RegisterTools(s)
	return s
}

// callTool invokes a registered tool's handler and returns the result text
// and whether it is an error result.
func callTool(t *testing.T, s *server.MCPServer, name string, args map[string]any) (string, bool) {
	t.Helper()
	entry := s.ListTools()[name]
	if entry == nil {
		t.Fatalf("tool %s is not registered", name)
	}
	res, err := entry.Handler(context.Background(), mcplib.CallToolRequest{Params: mcplib.CallToolParams{Name: name, Arguments: args}})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	text := ""
	for _, c := range res.Content {
		if tc, ok := c.(mcplib.TextContent); ok {
			text += tc.Text
		}
	}
	return text, res.IsError
}
