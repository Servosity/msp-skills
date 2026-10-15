// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cobratree

import (
	"strings"

	"cisco-duo-pp-cli/internal/mcp/bound"
	mcplib "github.com/mark3labs/mcp-go/mcp"
)

// cliFailureToolResult is the MCP result for a companion CLI run that exited
// non-zero. The generated handler returned only the error text, which quotes
// stderr whenever there is any, so a write command that printed its receipt
// to stdout before a later step failed (lockout fix after the bypass code was
// issued, phone swap after the phone was attached, enrollment chase after some
// emails went out) lost that receipt over MCP. The result stays IsError, with
// the error text first; the stdout receipt follows in its own content block.
// When stderr was empty the error text already quotes stdout, so it is not
// repeated.
func cliFailureToolResult(out CLICommandResult, err error) *mcplib.CallToolResult {
	result := boundedToolResultError(err.Error())
	stdout := strings.TrimSpace(out.Stdout)
	if stdout == "" || strings.Contains(err.Error(), bound.Text(stdout)) {
		return result
	}
	result.Content = append(result.Content, mcplib.NewTextContent(bound.Text(out.Stdout)))
	return result
}
