// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package mcp

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// localPathUsage matches flag help that names a path on this machine.
var localPathUsage = regexp.MustCompile(`(?i)\b(path to|file path|jsonl? file|markdown file)\b`)

// hand-fix (handfixes.json mcp-local-file-sources-blocked, ported from
// threatdown): no MCP tool may
// take a local file path, because the server account would read it.
// Walks the live registered surface, so a reprint that grows a new
// path-reading flag fails here.
func TestNoMCPToolTakesALocalFilePath(t *testing.T) {
	s := server.NewMCPServer("watchguard", "test")
	RegisterTools(s)
	for name, tool := range s.ListTools() {
		for prop, raw := range tool.Tool.InputSchema.Properties {
			desc := ""
			if m, ok := raw.(map[string]any); ok {
				desc, _ = m["description"].(string)
			}
			switch prop {
			case "input", "notes-file", "playbook-file", "playbook-notes-file":
				t.Errorf("tool %s exposes local-file parameter %q", name, prop)
			}
			if localPathUsage.MatchString(desc) {
				t.Errorf("tool %s parameter %q reads a local path: %q", name, prop, desc)
			}
		}
	}
}

func TestTeachPlaybookRefusesNotesFile(t *testing.T) {
	secret := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(secret, []byte("SENTINEL-NOTES-FILE"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Stub companion CLI that echoes its argv, so a forwarded path shows up.
	stub := filepath.Join(t.TempDir(), "watchguard-cli")
	if err := os.WriteFile(stub, []byte("#!/bin/sh\necho \"ARGV: $*\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WATCHGUARD_CLI_PATH", stub)
	s := server.NewMCPServer("watchguard", "test")
	RegisterTools(s)
	tool := s.GetTool("teach_playbook")
	if tool == nil {
		t.Fatal("teach_playbook tool not registered")
	}
	req := mcplib.CallToolRequest{}
	req.Params.Name = "teach_playbook"
	req.Params.Arguments = map[string]any{"query": "credential audit", "notes-file": secret}
	res, err := tool.Handler(context.Background(), req)
	if err != nil {
		if !strings.Contains(err.Error(), "notes-file") || strings.Contains(err.Error(), secret) {
			t.Fatalf("want a refusal naming notes-file, got %v", err)
		}
		return
	}
	if res == nil || !res.IsError {
		t.Fatalf("teach_playbook accepted notes-file: %#v", res)
	}
	if text := mcpTextContent(t, res); !strings.Contains(text, "notes-file") || strings.Contains(text, secret) {
		t.Fatalf("want a refusal naming notes-file and the path never forwarded, got %q", text)
	}
}

// import needs --input, which MCP refuses, so it is not an MCP tool.
func TestImportIsNotAnMCPTool(t *testing.T) {
	s := server.NewMCPServer("watchguard", "test")
	RegisterTools(s)
	if s.GetTool("import") != nil {
		t.Fatal("import is registered but can never succeed over MCP")
	}
}
