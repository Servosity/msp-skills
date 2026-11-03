// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package mcp

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// callWithArgvStub calls a registered tool against a companion CLI stub that
// echoes its argv, and returns what the CLI would have been run with.
func callWithArgvStub(t *testing.T, tool string, args map[string]any) string {
	t.Helper()
	// A #!/bin/sh stub cannot run on Windows; use a .bat there (as connectsecure does).
	stub, body := filepath.Join(t.TempDir(), "watchguard-cli"), "#!/bin/sh\necho \"ARGV: $*\"\n"
	if runtime.GOOS == "windows" {
		stub, body = stub+".bat", "@echo off\r\necho ARGV: %*\r\n"
	}
	if err := os.WriteFile(stub, []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WATCHGUARD_CLI_PATH", stub)
	s := server.NewMCPServer("watchguard", "test")
	RegisterTools(s)
	st := s.GetTool(tool)
	if st == nil {
		t.Fatalf("%s tool not registered", tool)
	}
	req := mcplib.CallToolRequest{}
	req.Params.Name = tool
	req.Params.Arguments = args
	res, err := st.Handler(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	return mcpTextContent(t, res)
}

// hand-fix (handfixes.json mcp-bool-false-default-true, ported from
// connectsecure): an explicit false from an MCP client reaches a bool flag
// whose default is true. teach --quiet defaults to true, so quiet:false was
// dropped from argv and the call ran quiet anyway.
func TestExplicitFalseReachesDefaultTrueFlag(t *testing.T) {
	got := callWithArgvStub(t, "teach", map[string]any{"query": "firmware lag", "quiet": false, "json": false})
	if !strings.Contains(got, "--quiet=false") {
		t.Fatalf("teach quiet:false never reached the CLI: %q", got)
	}
	if strings.Contains(got, "--json") {
		t.Fatalf("a default-false flag must stay omitted when false: %q", got)
	}
}

// hand-fix (handfixes.json tail-redacts-secrets): tail's poll loop never
// returns over MCP, so it is not an MCP tool (also not in read-only mode).
func TestTailIsNotAnMCPTool(t *testing.T) {
	for _, ro := range []string{"", "1"} {
		t.Setenv("WATCHGUARD_MCP_READ_ONLY", ro)
		s := server.NewMCPServer("watchguard", "test")
		RegisterTools(s)
		if s.GetTool("tail") != nil {
			t.Fatalf("tail is registered as an MCP tool (WATCHGUARD_MCP_READ_ONLY=%q)", ro)
		}
	}
}
