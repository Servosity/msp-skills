// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"eset-protect-pp-cli/internal/cliutil/credvariants"
	"eset-protect-pp-cli/internal/secrets"
	"eset-protect-pp-cli/internal/store"
	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// buildEchoCLI builds a stand-in companion CLI that prints the contents of
// $ECHO_FILE to stdout and stderr, so a mirrored tool's result is text no
// in-process masking touched before the MCP boundary.
func buildEchoCLI(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	src := filepath.Join(dir, "echocli.go")
	body := `package main

import (
	"fmt"
	"os"
)

func main() {
	b, _ := os.ReadFile(os.Getenv("ECHO_FILE"))
	fmt.Fprintln(os.Stdout, string(b))
	fmt.Fprintln(os.Stderr, string(b))
}
`
	if err := os.WriteFile(src, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(dir, "eset-protect-cli-echo")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	cmd := exec.Command("go", "build", "-o", bin, src)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build echo CLI: %v\n%s", err, out)
	}
	return bin
}

func callTool(t *testing.T, s *server.MCPServer, name string, args map[string]any) []byte {
	t.Helper()
	msg, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": map[string]any{"name": name, "arguments": args}})
	resp := s.HandleMessage(context.Background(), msg)
	out, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("%s: marshal response: %v", name, err)
	}
	return out
}

// TestMCPToolResultsNeverCarryASecret is the MCP half of the boundary
// property test (handfixes.json: secret-registry-masks-every-boundary).
// Random secrets registered in the server process are echoed back, in every
// spelling, through every kind of tool: eset-protect_execute (an API error
// body and a successful body), the isolate_a_machine intent and a cobratree
// mirror (the companion CLI's stdout and stderr), sql and search over a
// mirror row holding the secret, and a tool that returns a Go error. Every
// JSON-RPC response goes through the server's dispatch, so the outermost
// middleware is under test; none may let the oracle recover the secret.
func TestMCPToolResultsNeverCarryASecret(t *testing.T) {
	t.Setenv("ESET_PROTECT_MCP_READ_ONLY", "")
	var mu sync.Mutex
	echo := ""
	esetFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		text := echo
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("pageToken") == "ok" {
			_ = json.NewEncoder(w).Encode(map[string]any{"devices": []map[string]string{{"uuid": "00000000-0000-0000-0000-000000000001", "displayName": text}}, "nextPageToken": ""})
			return
		}
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(map[string]string{"message": "Rejected credential " + text})
	})
	echoCLI := buildEchoCLI(t)
	echoFile := filepath.Join(t.TempDir(), "echo.txt")
	t.Setenv("ECHO_FILE", echoFile)
	t.Setenv("ESET_PROTECT_CLI_PATH", echoCLI)
	savedRecipe, savedRecipeErr := recipeCLIPath, recipeCLIPathErr
	recipeCLIPath, recipeCLIPathErr = echoCLI, nil
	t.Cleanup(func() { recipeCLIPath, recipeCLIPathErr = savedRecipe, savedRecipeErr })

	s := server.NewMCPServer("eset-protect", "test")
	RegisterTools(s)
	s.AddTool(mcplib.NewTool("zz_error_echo"), func(context.Context, mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		mu.Lock()
		defer mu.Unlock()
		return nil, errors.New("tool failed: " + echo)
	})
	mirror := ""
	for name, tool := range s.ListTools() {
		if tool.Tool.Meta != nil && tool.Tool.Meta.AdditionalFields["pp:tenant-gate"] == "child-cli" && strings.HasPrefix(name, "fleet") {
			mirror = name
			break
		}
	}
	if mirror == "" {
		t.Fatal("no cobratree mirror tool to call")
	}

	path, err := mcpDBPath()
	if err != nil {
		t.Fatal(err)
	}
	r := credvariants.NewRand(71)
	for trial := 0; trial < 4; trial++ {
		secret := credvariants.RandomSecret(r)
		secrets.Register(secret)
		for _, f := range credvariants.SecretForms(secret) {
			mu.Lock()
			echo = "FRONTDESK-01 " + f.Text
			mu.Unlock()
			if err := os.WriteFile(echoFile, []byte(`{"name":"FRONTDESK-01 `+f.Text+`"}`), 0o600); err != nil {
				t.Fatal(err)
			}
			db, err := store.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			row, _ := json.Marshal(map[string]string{"uuid": "00000000-0000-0000-0000-000000000001", "displayName": "FRONTDESK-01 " + f.Text})
			if _, _, err := db.UpsertBatch("devices", []json.RawMessage{row}); err != nil {
				t.Fatal(err)
			}
			_ = db.Close()
			for _, call := range []struct {
				name string
				args map[string]any
			}{
				{"eset-protect_execute", map[string]any{"endpoint_id": "devices.list", "params": map[string]any{}}},
				{"eset-protect_execute", map[string]any{"endpoint_id": "devices.list", "params": map[string]any{"pageToken": "ok"}}},
				{"isolate_a_machine", map[string]any{"slug": "FRONTDESK-01"}},
				{mirror, map[string]any{}},
				{"sql", map[string]any{"query": "SELECT data FROM resources WHERE resource_type='devices'"}},
				{"search", map[string]any{"query": "FRONTDESK"}},
				{"zz_error_echo", map[string]any{}},
			} {
				out := callTool(t, s, call.name, call.args)
				if credvariants.SecretRecoverable(out, secret) {
					t.Fatalf("trial %d, form %s, tool %s: the secret reached the MCP host\nsecret %q\n   out %s", trial, f.Name, call.name, secret, out)
				}
			}
		}
	}
}

// TestMaskToolResultCoversEveryContentKind: text, embedded text resources,
// resource links and structured content are all masked.
func TestMaskToolResultCoversEveryContentKind(t *testing.T) {
	const secret = "content-kind-secret-3X5C"
	secrets.Register(secret)
	res := &mcplib.CallToolResult{
		Content: []mcplib.Content{
			mcplib.TextContent{Type: "text", Text: "a " + secret},
			&mcplib.TextContent{Type: "text", Text: "b " + secret},
			mcplib.EmbeddedResource{Type: "resource", Resource: mcplib.TextResourceContents{URI: "x://" + secret, Text: "c " + secret}},
			mcplib.ResourceLink{Type: "resource_link", URI: "x://" + secret, Name: secret, Description: secret},
		},
		StructuredContent: map[string]any{"message": "d " + secret},
	}
	maskToolResult(res)
	out, _ := json.Marshal(res)
	if strings.Contains(string(out), "3X5C") {
		t.Fatalf("a content kind kept the secret: %s", out)
	}
}
