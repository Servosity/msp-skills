// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored: guards handfixes.json mcp-blocks-local-file-inputs.

package mcp

import (
	"regexp"
	"testing"

	"github.com/mark3labs/mcp-go/server"
)

// TestNoRegisteredToolTakesALocalFilePath checks the real tool surface: no
// tool may expose a parameter named like a local path to read (*-file, input)
// or one of the multipart upload flags, so a dropped fileInputFlags entry
// fails here per command, not only by name.
func TestNoRegisteredToolTakesALocalFilePath(t *testing.T) {
	s := server.NewMCPServer("file-inputs", "test")
	RegisterTools(s)
	pathLike := regexp.MustCompile(`(^|-)file$|^input$|^i$|^ssl-(cert|privkey)$|^(company-logo|cover-page|footer-image|header-image|file-name)$`)
	checked := 0
	for name, entry := range s.ListTools() {
		for prop := range entry.Tool.InputSchema.Properties {
			checked++
			if pathLike.MatchString(prop) {
				t.Errorf("tool %s exposes local-path parameter %q", name, prop)
			}
		}
	}
	if checked == 0 {
		t.Fatal("no tool properties registered")
	}
}

// TestImportIsNotAnMCPTool: import reads only a local file, which MCP may not
// name, so a registered import tool could never succeed.
func TestImportIsNotAnMCPTool(t *testing.T) {
	s := server.NewMCPServer("file-inputs", "test")
	RegisterTools(s)
	if _, ok := s.ListTools()["import"]; ok {
		t.Fatal("import is registered as an MCP tool")
	}
}
