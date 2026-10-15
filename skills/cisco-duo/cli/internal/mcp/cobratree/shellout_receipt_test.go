// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cobratree

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	mcplib "github.com/mark3labs/mcp-go/mcp"
)

// writeReceiptHelper fakes a write command that completed some steps, wrote
// its receipt to stdout, then failed a later step: the receipt goes to
// stdout, the error to stderr, and the process exits non-zero. This is the
// shape of lockout fix (bypass code issued, activation failed), phone swap
// and enrollment chase.
func writeReceiptHelper(t *testing.T, stdout string) string {
	t.Helper()
	dir := t.TempDir()
	receipt := filepath.Join(dir, "receipt.json")
	if err := os.WriteFile(receipt, []byte(stdout), 0o600); err != nil {
		t.Fatalf("write receipt: %v", err)
	}
	if runtime.GOOS == "windows" {
		path := filepath.Join(dir, "helper.bat")
		body := "@echo off\r\ntype \"" + receipt + "\"\r\necho Error: send activation: HTTP 500 1>&2\r\nexit /b 5\r\n"
		if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
			t.Fatalf("write helper: %v", err)
		}
		return path
	}
	path := filepath.Join(dir, "helper.sh")
	body := "#!/bin/sh\ncat '" + receipt + "'\necho 'Error: send activation: HTTP 500' >&2\nexit 5\n"
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("write helper: %v", err)
	}
	return path
}

// Codex merge review (P2): a command that fails after some steps succeeded
// writes its receipt to stdout, but the shell-out handler returned only the
// error text (which quotes stderr), so an MCP caller lost the receipt and with
// it a bypass code that was already issued. The result must stay an error and
// must carry the receipt.
func TestShellOutKeepsReceiptWhenCommandFails(t *testing.T) {
	receipt := `{"applied":true,"steps":[{"action":"bypass","status":"done","codes":["123456789"]},{"action":"reactivate","status":"failed"}]}` + "\n"
	bin := writeReceiptHelper(t, receipt)
	handler := shellOutToCLI(func() (string, error) { return bin, nil }, nil, map[string]bool{}, map[string]bool{}, nil, false, nil)

	result, err := handler(context.Background(), mcplib.CallToolRequest{})
	if err != nil {
		t.Fatalf("handler returned transport error: %v", err)
	}
	if !result.IsError {
		t.Fatalf("a failed command must stay an MCP error; got success: %s", toolResultText(result))
	}
	if !strings.Contains(toolResultText(result), "HTTP 500") {
		t.Fatalf("first content block lost the error text: %q", toolResultText(result))
	}
	var found bool
	for i := range result.Content {
		text := toolResultContentText(result, i)
		var payload map[string]any
		if json.Unmarshal([]byte(text), &payload) == nil && payload["applied"] == true {
			found = true
			if !strings.Contains(text, "123456789") {
				t.Fatalf("receipt block lost the issued code: %q", text)
			}
		}
	}
	if !found {
		t.Fatalf("no content block carries the stdout receipt as JSON; content: %#v", result.Content)
	}
}

// A failure with nothing on stdout keeps the single error block, and a failure
// whose only output is stdout (already quoted in the error text) is not
// repeated in a second block.
func TestShellOutFailureWithoutSeparateReceiptAddsNoBlock(t *testing.T) {
	for _, mode := range []string{"fail-stderr", "fail-stdout"} {
		t.Run(mode, func(t *testing.T) {
			bin := writeShelloutHelper(t, mode)
			handler := shellOutToCLI(func() (string, error) { return bin, nil }, nil, map[string]bool{}, map[string]bool{}, nil, false, nil)
			result, err := handler(context.Background(), mcplib.CallToolRequest{})
			if err != nil {
				t.Fatalf("handler returned transport error: %v", err)
			}
			if !result.IsError {
				t.Fatalf("handler returned success: %s", toolResultText(result))
			}
			if len(result.Content) != 1 {
				t.Fatalf("got %d content blocks, want 1: %#v", len(result.Content), result.Content)
			}
		})
	}
}
