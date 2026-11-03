// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored: onboard's --template names a local file it reads (and --init
// writes), so the MCP mirror must neither advertise nor forward it. The fleet
// gate check_mcp_file_flags.py proves the same end to end against the binary.

package cobratree_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"hudu-pp-cli/internal/cli"
	"hudu-pp-cli/internal/cliutil"
	"hudu-pp-cli/internal/mcp/cobratree"
)

func TestOnboardTemplateIsNotAnMCPArgument(t *testing.T) {
	cmd, _, err := cli.RootCmd().Find([]string{"onboard"})
	if err != nil || cmd == nil || cmd.Name() != "onboard" {
		t.Fatalf("onboard command not found: %v", err)
	}
	if !cobratree.InputFlagBlocked(cmd, "template") {
		t.Error("onboard --template is a file read reachable from an MCP tool argument")
	}
	if !cobratree.DestinationFlagBlocked(cmd, "template") {
		t.Error("onboard --template (with --init) is a file write reachable from an MCP tool argument")
	}
	// --init takes no path, but it writes ~/.config/hudu-cli/templates/<name>.json
	// (codex r1: MCP host-file write class), so it is a write sink as well.
	if !cobratree.DestinationFlagBlocked(cmd, "init") {
		t.Error("onboard --init writes a host file and is reachable from an MCP tool argument")
	}
	// The gate must stay narrow: an ordinary onboard flag is still forwarded.
	if cobratree.InputFlagBlocked(cmd, "company") || cobratree.DestinationFlagBlocked(cmd, "company") {
		t.Error("onboard --company is not a path and must stay an MCP argument")
	}
}

// TestOnboardHostFileArgumentsRefusedOverMCP drives the real mirrored onboard
// tool (codex r1, MCP host-file write class): every argument that leads onboard
// to a host-file read or write is absent from the tool schema and refused at
// call time, before the companion CLI ever runs. `{init:true}` used to reach
// onboardInit and overwrite ~/.config/hudu-cli/templates/msp-standard.json.
func TestOnboardHostFileArgumentsRefusedOverMCP(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the recording companion CLI stand-in is a POSIX shell script")
	}
	t.Setenv(cliutil.MCPReadOnlyEnvVar, "")
	dir := t.TempDir()
	marker := filepath.Join(dir, "companion-ran")
	bin := filepath.Join(dir, "hudu-cli")
	script := "#!/bin/sh\necho \"$@\" >> '" + marker + "'\necho '{}'\n"
	if err := os.WriteFile(bin, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}

	s := server.NewMCPServer("test", "0.0.0")
	cobratree.RegisterAll(s, cli.RootCmd(), func() (string, error) { return bin, nil })
	var entry *server.ServerTool
	for _, tool := range s.ListTools() {
		if tool.Tool.Meta != nil && tool.Tool.Meta.AdditionalFields["pp:cli-command"] == "onboard" {
			entry = tool
		}
	}
	if entry == nil {
		t.Fatal("onboard is not mirrored as an MCP tool")
	}
	props := entry.Tool.InputSchema.Properties
	for _, sink := range []string{"init", "template"} {
		if _, ok := props[sink]; ok {
			t.Errorf("onboard tool schema advertises host-file argument %q", sink)
		}
	}
	if _, ok := props["company"]; !ok {
		t.Error("onboard tool schema lost the ordinary --company argument")
	}

	for _, args := range []map[string]any{
		{"init": true},
		{"init": true, "company": float64(42)},
		{"template": "/etc/hosts", "company": float64(42)},
	} {
		result, err := entry.Handler(context.Background(), mcplib.CallToolRequest{Params: mcplib.CallToolParams{Arguments: args}})
		if err != nil {
			t.Fatalf("%v: transport error %v", args, err)
		}
		if result == nil || !result.IsError {
			t.Errorf("%v: accepted a host-file argument over MCP", args)
		}
		if ran, statErr := os.ReadFile(marker); statErr == nil {
			t.Errorf("%v: the companion CLI ran with %q", args, ran)
			_ = os.Remove(marker)
		}
	}
}
