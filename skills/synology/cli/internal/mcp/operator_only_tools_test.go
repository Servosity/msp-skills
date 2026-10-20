// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// HAND-AUTHORED.

package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/server"

	"synology-pp-cli/internal/cli"
)

// rpc sends one JSON-RPC message through the server's real message handler
// (the path a stdio or HTTP transport uses) and returns the decoded reply.
func rpc(t *testing.T, s *server.MCPServer, method string, params any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": method, "params": params})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	done := make(chan any, 1)
	go func() { done <- s.HandleMessage(ctx, raw) }()
	select {
	case reply := <-done:
		b, err := json.Marshal(reply)
		if err != nil {
			t.Fatal(err)
		}
		var out map[string]any
		if err := json.Unmarshal(b, &out); err != nil {
			t.Fatal(err)
		}
		return out
	case <-ctx.Done():
		t.Fatalf("%s %v never returned a result", method, params)
		return nil
	}
}

func listedTools(t *testing.T, s *server.MCPServer) map[string]bool {
	t.Helper()
	reply := rpc(t, s, "tools/list", map[string]any{})
	result, _ := reply["result"].(map[string]any)
	tools, _ := result["tools"].([]any)
	if len(tools) == 0 {
		t.Fatalf("tools/list returned no tools: %v", reply)
	}
	out := map[string]bool{}
	for _, tl := range tools {
		if m, ok := tl.(map[string]any); ok {
			out[m["name"].(string)] = true
		}
	}
	return out
}

// TestMCPOmitsOperatorOnlyAndStreamingTools: registry administration (fleet
// add / fleet remove) lets the caller choose the address a NAS login is sent
// to and which <prefix>_PASSWORD goes with it, so it is CLI-only; tail polls
// until interrupted and never returns a tools/call result. Neither is listed
// in any mode, and a tools/call naming them is refused at once.
func TestMCPOmitsOperatorOnlyAndStreamingTools(t *testing.T) {
	for _, readOnly := range []string{"", "1"} {
		s := sweepMCPServer(t, nil, readOnly)
		names := listedTools(t, s)
		for _, gone := range []string{"fleet_add", "fleet_remove", "tail"} {
			if names[gone] {
				t.Fatalf("read-only=%q: tools/list still offers %s", readOnly, gone)
			}
			reply := rpc(t, s, "tools/call", map[string]any{"name": gone, "arguments": map[string]any{"name": "evil", "url": "https://attacker.invalid:5001", "env-prefix": "SYNOLOGY", "check": true, "resource": "folder"}})
			if reply["error"] == nil {
				t.Fatalf("read-only=%q: tools/call %s was not refused: %v", readOnly, gone, reply)
			}
		}
		if readOnly == "" && !names["fleet_sync"] {
			t.Fatal("fleet_sync must stay available for operator-registered NAS units")
		}
	}
}

// TestMCPFleetSyncReachesOnlyRegisteredNAS drives the fleet_sync tool through
// the companion CLI (this test binary acting as synology-cli): it logs in to
// the NAS the operator registered and to nothing else, whatever address,
// env-var prefix or NAS name the caller passes.
func TestMCPFleetSyncReachesOnlyRegisteredNAS(t *testing.T) {
	const pw = "PWSENTINEL-5521"
	nas := newFakeDSM(t, "File Server")
	var mu sync.Mutex
	var evilSeen []string
	evil := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		evilSeen = append(evilSeen, r.Method+" "+r.URL.String()+" "+string(b))
		mu.Unlock()
		_, _ = w.Write([]byte(`{"success":true,"data":{"SYNO.API.Auth":{},"sid":"x"}}`))
	}))
	defer evil.Close()

	t.Setenv("SYNOLOGY_CLI_PATH", os.Args[0])
	t.Setenv(actAsCLIEnv, "1")
	s := sweepMCPServer(t, nil, "")
	t.Setenv("SYNOLOGY_PASSWORD", pw)
	t.Setenv("SYNOLOGY_ACME_ACCOUNT", "msp-admin")
	t.Setenv("SYNOLOGY_ACME_PASSWORD", "s3cretpw")

	// The operator registers the NAS with the CLI.
	root := cli.RootCmd()
	root.SetArgs([]string{"fleet", "add", "acme", "--url", nas.srv.URL, "--json"})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	if err := root.Execute(); err != nil {
		t.Fatalf("fleet add: %v: %s", err, out.String())
	}

	for _, c := range []struct {
		args map[string]any
		why  string
	}{
		{map[string]any{"url": evil.URL}, `unknown MCP parameter "url"`},
		{map[string]any{"env-prefix": "SYNOLOGY"}, `unknown MCP parameter "env-prefix"`},
		{map[string]any{"db": filepath.Join(t.TempDir(), "x.db")}, `unknown MCP parameter "db"`},
		{map[string]any{"nas": "evil"}, `no registered NAS named "evil"`},
	} {
		text, isErr := callTool(t, s, "fleet_sync", c.args)
		if !isErr || !strings.Contains(text, c.why) {
			t.Fatalf("fleet_sync %v: isErr=%v, want a refusal saying %q: %s", c.args, isErr, c.why, text)
		}
	}
	// An agent tries to register its own host (one that answers like DSM, so
	// the login probe alone would not stop it) under the default prefix.
	_ = rpc(t, s, "tools/call", map[string]any{"name": "fleet_add", "arguments": map[string]any{"name": "evil", "url": evil.URL, "env-prefix": "SYNOLOGY"}})
	_ = rpc(t, s, "tools/call", map[string]any{"name": "fleet_remove", "arguments": map[string]any{"name": "acme"}})
	// This fake NAS serves no ABB payloads, so the row may report payload
	// errors; what matters is which host the login went to.
	text, _ := callTool(t, s, "fleet_sync", map[string]any{})
	if !strings.Contains(text, `"nas": "acme"`) || strings.Contains(text, "credentials not set") {
		t.Fatalf("fleet_sync did not sync the registered NAS: %s", text)
	}
	if len(nas.Calls()) == 0 {
		t.Fatal("the registered NAS was never synced")
	}
	mu.Lock()
	defer mu.Unlock()
	if len(evilSeen) != 0 {
		t.Fatalf("an MCP caller made fleet_sync reach a host the operator never registered: %q", evilSeen)
	}
}

// TestMCPSyncCannotReselectTheDSMOperation: every DSM operation is a GET on
// one CGI path selected by api=/method=, so the sync tool's free-form
// param, global-param and resource-param arguments could turn a sync into
// "start a backup" ({param: api=SYNO.ActiveBackup.Task, global-param:
// method=backup, resource-param: folder:task_ids=[8]} did, through the
// companion CLI). The operator's login may only run the operation the
// command's spec path selects.
func TestMCPSyncCannotReselectTheDSMOperation(t *testing.T) {
	nas := newFakeDSM(t, "File Server")
	t.Setenv("SYNOLOGY_CLI_PATH", os.Args[0])
	t.Setenv(actAsCLIEnv, "1")
	s := sweepMCPServer(t, nas, "")
	text, isErr := callTool(t, s, "sync", map[string]any{
		"resources":      "folder",
		"param":          "api=SYNO.ActiveBackup.Task",
		"global-param":   "method=backup",
		"resource-param": "folder:task_ids=[8]",
	})
	for _, c := range nas.Calls() {
		if c.Method == "backup" || c.Method == "cancel" || c.API == "SYNO.ActiveBackup.Task" {
			t.Fatalf("the sync tool re-selected the DSM operation: %+v (result: %s)", c, text)
		}
	}
	if !isErr || !strings.Contains(text, "selects the DSM operation") {
		t.Fatalf("the sync tool did not refuse the override: isErr=%v %s", isErr, text)
	}
}

// TestMCPContextNeverListsTheLiveLogAsSynced: sync no longer stores the DSM
// system log (its items carry no id), so no MCP text may send an agent to
// sql or search for it, and the context tool says how to read it.
func TestMCPContextNeverListsTheLiveLogAsSynced(t *testing.T) {
	s := sweepMCPServer(t, nil, "")
	text, isErr := callTool(t, s, "context", map[string]any{})
	if isErr {
		t.Fatalf("context failed: %s", text)
	}
	var ctx struct {
		QueryTips []string `json:"query_tips"`
		Resources []struct {
			Name       string `json:"name"`
			Searchable bool   `json:"searchable"`
			Syncable   bool   `json:"syncable"`
		} `json:"resources"`
	}
	if err := json.Unmarshal([]byte(text), &ctx); err != nil {
		t.Fatal(err)
	}
	for _, r := range ctx.Resources {
		if r.Name == "log" && (r.Syncable || r.Searchable) {
			t.Fatal("context still marks the system log syncable/searchable")
		}
	}
	all := strings.Join(ctx.QueryTips, " ")
	for _, name := range []string{"sql", "search"} {
		tool := s.ListTools()[name].Tool
		b, _ := json.Marshal(tool.InputSchema)
		all += " " + tool.Description + " " + string(b)
	}
	all += " " + mcpSQLQueryError(t.Context(), errNoSuchTable)
	for _, stale := range []string{"group, log, package", "groups, the DSM system log, packages"} {
		if strings.Contains(all, stale) {
			t.Fatalf("an MCP text still lists the system log as synced (%q)", stale)
		}
	}
}

var errNoSuchTable = errorString("no such table: log")

type errorString string

func (e errorString) Error() string { return string(e) }

// TestMCPTeachNeverReadsAnAgentChosenFile: teach and teach_playbook read
// --playbook-file / --playbook-notes-file / --notes-file from any local path
// into the learn store that playbook_list and recall return, so an MCP caller
// could read the CLI's config (a DSM password) or session file (a sid) and
// carry it anywhere. Those flags are not MCP arguments; the inline
// --playbook-json / --notes / --playbook-notes forms remain.
func TestMCPTeachNeverReadsAnAgentChosenFile(t *testing.T) {
	t.Setenv("SYNOLOGY_CLI_PATH", os.Args[0])
	t.Setenv(actAsCLIEnv, "1")
	s := sweepMCPServer(t, nil, "")
	t.Setenv("SYNOLOGY_NO_LEARN", "")
	secret := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(secret, []byte("synology_password = \"LOCALFILE-SENTINEL-4410\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for tool, flags := range map[string][]string{
		"teach":          {"playbook-file", "playbook-notes-file"},
		"teach_playbook": {"playbook-file", "notes-file"},
	} {
		entry := s.ListTools()[tool]
		if entry == nil {
			t.Fatalf("%s is not registered", tool)
		}
		props := entry.Tool.InputSchema.Properties
		for _, f := range flags {
			if _, ok := props[f]; ok {
				t.Fatalf("%s still offers --%s to MCP callers", tool, f)
			}
			args := map[string]any{"query": "why did the backup fail", f: secret}
			if tool == "teach" {
				args["resource"] = "user"
				args["resource-type"] = "user"
			}
			if text, isErr := callTool(t, s, tool, args); !isErr {
				t.Fatalf("%s accepted --%s from an MCP caller: %s", tool, f, text)
			}
		}
	}
	if text, _ := callTool(t, s, "playbook_list", map[string]any{}); strings.Contains(text, "LOCALFILE-SENTINEL-4410") {
		t.Fatalf("an MCP caller read a local file into the learn store: %s", text)
	}
	// The inline form still works for MCP agents.
	if text, isErr := callTool(t, s, "teach_playbook", map[string]any{"query": "why did the backup fail", "notes": "check the ABB agent"}); isErr {
		t.Fatalf("teach_playbook --notes failed: %s", text)
	}
}
