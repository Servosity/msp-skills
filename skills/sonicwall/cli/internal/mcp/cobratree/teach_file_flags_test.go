// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/sonicwall/handfixes.json under the
// "mcp-teach-local-file-flags-blocked" entry. External test package so it can
// import internal/cli even if internal/cli ever imports internal/mcp.

package cobratree_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"sonicwall-pp-cli/internal/cli"
	"sonicwall-pp-cli/internal/cliutil"
	"sonicwall-pp-cli/internal/mcp/cobratree"
)

// TestTeachFileFlagsNotSettableOverMCP drives the real MCP tool surface built
// from the real command tree. teach and teach-playbook read these flags' files
// from the local disk into the learn store, where playbook_list and recall
// return them, so over MCP each one is a read of any file the server account
// can reach. Each flag must be missing from the tool schema and refused at
// call time before the companion CLI is ever spawned.
func TestTeachFileFlagsNotSettableOverMCP(t *testing.T) {
	t.Setenv(cliutil.MCPReadOnlyEnvVar, "")
	root := cli.RootCmd()
	s := server.NewMCPServer("test", "0.0.0")
	// A CLI path that does not exist: a flag that is NOT refused reaches the
	// spawn step and fails there with a different message.
	missingCLI := filepath.Join(t.TempDir(), "no-such-cli")
	cobratree.RegisterAll(s, root, func() (string, error) { return missingCLI, nil })
	tools := s.ListTools()

	cases := []struct {
		command string
		tool    string
		flags   []string
	}{
		{"teach", "teach", []string{"playbook-file", "playbook-notes-file"}},
		{"teach-playbook", "teach_playbook", []string{"playbook-file", "notes-file"}},
	}
	for _, c := range cases {
		cmd := cobratree.CommandAtPath(root, []string{c.command})
		if cmd == nil {
			t.Fatalf("command %q not found; update this test", c.command)
		}
		entry := tools[c.tool]
		if entry == nil {
			t.Fatalf("MCP tool %q missing; update this test", c.tool)
		}
		for _, name := range c.flags {
			if cmd.Flags().Lookup(name) == nil {
				t.Fatalf("%s no longer declares --%s; update this test", c.command, name)
			}
			if !cobratree.DestinationFlagBlocked(cmd, name) {
				t.Errorf("%s --%s reads a local file and must be blocked over MCP", c.command, name)
			}
			if _, ok := entry.Tool.InputSchema.Properties[name]; ok {
				t.Errorf("MCP tool %s advertises --%s in its input schema", c.tool, name)
			}
			result, err := entry.Handler(context.Background(), mcplib.CallToolRequest{Params: mcplib.CallToolParams{
				Arguments: map[string]any{"query": "probe", name: "/home/someone/.config/vendor/credentials.toml"},
			}})
			if err != nil {
				t.Fatalf("%s: handler returned a transport error: %v", c.tool, err)
			}
			text := ""
			if result != nil {
				for _, block := range result.Content {
					if tc, ok := block.(mcplib.TextContent); ok {
						text += tc.Text
					}
				}
			}
			want := "unknown MCP parameter \"" + name + "\""
			if result == nil || !result.IsError || !strings.Contains(text, want) {
				t.Errorf("MCP tool %s accepted --%s (response %q, want an error containing %q)", c.tool, name, text, want)
			}
		}
	}

	// The fix must not swallow the inline equivalents an MCP agent is meant
	// to use instead.
	for _, inline := range []string{"playbook-json", "playbook-notes"} {
		if _, ok := tools["teach"].Tool.InputSchema.Properties[inline]; !ok {
			t.Errorf("MCP tool teach lost its inline --%s parameter", inline)
		}
	}
}
