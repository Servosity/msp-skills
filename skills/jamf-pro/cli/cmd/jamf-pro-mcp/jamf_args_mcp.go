// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package main

// jamf-pro handfix (response-credentials-held): the kit holds the MCP arguments
// Declarations names. jamf-pro_execute also takes request bodies whose
// credential members sit at any depth, so every argument named or shaped like a
// credential is held by the one shared walker before the call runs. The kit's
// MCPTools wraps this, and is what checks each result.

import (
	"context"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"jamf-pro-pp-cli/internal/cliutil"
)

func holdToolArguments(next server.ToolHandlerFunc) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		cliutil.HoldCredentialValues(req.GetArguments())
		return next(ctx, req)
	}
}
