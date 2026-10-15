// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored (handfixes.json: mcp-partial-failure-keeps-receipt).

package cobratree

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	mcplib "github.com/mark3labs/mcp-go/mcp"
)

// TestShellOutFailureKeepsStdoutReceipt: a command that exits non-zero with a
// receipt on stdout and an error on stderr (requote --confirm with a failed
// line) must hand both to the MCP caller.
func TestShellOutFailureKeepsStdoutReceipt(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell helper")
	}
	bin := filepath.Join(t.TempDir(), "helper.sh")
	body := "#!/bin/sh\nprintf '{\"line_failures\":[{\"line\":2,\"may_have_been_written\":true}]}\\n'\necho 'Error: 1 of 2 lines failed' >&2\nexit 6\n"
	if err := os.WriteFile(bin, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	handler := shellOutToCLI(func() (string, error) { return bin, nil }, nil, map[string]bool{}, map[string]bool{}, nil, false, nil)
	result, err := handler(context.Background(), mcplib.CallToolRequest{})
	if err != nil || !result.IsError {
		t.Fatalf("want a tool error, got err=%v result=%v", err, result)
	}
	text := toolResultText(result)
	if !strings.Contains(text, "1 of 2 lines failed") || !strings.Contains(text, `"may_have_been_written":true`) {
		t.Fatalf("error result dropped stderr or the stdout receipt: %s", text)
	}
}
