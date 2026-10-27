// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cobratree

import (
	"context"
	"fmt"
	"strings"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"
)

// PositionalReadSinksAnnotation marks a command whose positional arguments are
// read as a local file when they start with "@" (raw query's document). It is
// the read counterpart of mcp:write-positionals, which press 4.34.0 has; the
// press has no read form. Over MCP an "@path" would name a file on the machine
// running the server, so the tool refuses any "@" positional before the CLI
// runs, read-only or not.
const PositionalReadSinksAnnotation = "mcp:read-positionals"

// refuseAtPathPositionals wraps a shell-out handler for a command carrying
// PositionalReadSinksAnnotation. Every positional the call can supply is
// checked: each structured positional field and every token of the args
// field, plus the args field as a whole (one positional is passed unsplit).
func refuseAtPathPositionals(cmd *cobra.Command, positionals []positionalArg, next server.ToolHandlerFunc) server.ToolHandlerFunc {
	if cmd == nil || strings.TrimSpace(cmd.Annotations[PositionalReadSinksAnnotation]) == "" {
		return next
	}
	return func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		args := req.GetArguments()
		var values []string
		for _, p := range positionals {
			if s, ok := args[p.InputName].(string); ok {
				values = append(values, s)
			}
		}
		if raw, ok := args["args"].(string); ok {
			values = append(values, raw)
			values = append(values, SplitShellArgs(raw)...)
		}
		for _, v := range values {
			if strings.HasPrefix(strings.TrimSpace(v), "@") {
				return boundedToolResultError(fmt.Sprintf("%q names a local file on the machine running the MCP server; over MCP pass the value inline, not an @path", strings.TrimSpace(v))), nil
			}
		}
		return next(ctx, req)
	}
}
