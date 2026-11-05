// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package main

// prtg handfix (response-credentials-held): the kit holds the MCP arguments
// Declarations names. prtg_execute also takes request bodies whose credential
// members sit at any depth (a session login password in params), so every
// argument named like a credential is held by the one shared walker before the
// call runs. The kit's MCPTools wraps this, and is what checks each result.
//
// prtg handfix (bulk-action-outcomes): a write this call sent whose result has
// any failed item fails the call, with per-item detail.

import (
	"context"
	"strings"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"prtg-pp-cli/internal/cliutil"
)

func holdToolArguments(next server.ToolHandlerFunc) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		cliutil.HoldOperatorValues(req.GetArguments())
		ctx, actions := cliutil.WithActionFailures(ctx)
		res, err := next(ctx, req)
		if failures := actions.Summaries(); len(failures) > 0 && err == nil {
			if res == nil {
				res = &mcplib.CallToolResult{}
			}
			res.IsError = true
			res.Content = append([]mcplib.Content{mcplib.NewTextContent("bulk action failed: " + strings.Join(failures, "; "))}, res.Content...)
		}
		return res, err
	}
}
