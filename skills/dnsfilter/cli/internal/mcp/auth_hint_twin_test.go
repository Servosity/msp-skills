// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package mcp

import (
	"os"
	"strings"
	"testing"
)

// TestMCPAuthHintsHaveWindowsTwin guards handfixes.json:
// mcp-auth-hints-windows-twin. Every POSIX `export DNSFILTER_API_KEY=` hint the
// MCP server returns is followed by its PowerShell form; most MSPs run Windows.
func TestMCPAuthHintsHaveWindowsTwin(t *testing.T) {
	src, err := os.ReadFile("tools.go")
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(src), "\n")
	posix := 0
	for i, line := range lines {
		if !strings.Contains(line, "export DNSFILTER_API_KEY=") {
			continue
		}
		posix++
		if i+1 >= len(lines) || !strings.Contains(lines[i+1], "$env:DNSFILTER_API_KEY") {
			t.Errorf("tools.go:%d: POSIX auth hint has no PowerShell twin on the next line", i+1)
		}
	}
	if posix == 0 {
		t.Fatal("no auth hints found; the scan is not looking at the right file")
	}
}
