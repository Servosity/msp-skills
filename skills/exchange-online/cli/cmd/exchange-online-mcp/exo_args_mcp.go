// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package main

// Hand-fix (handfixes.json credkit-wiring): the kit holds the MCP arguments Declarations
// names. exchange-online_execute also takes request bodies whose credential
// members sit at any depth (a Password inside a nested parameter), so every
// argument named or shaped like a credential is held by the one shared walker
// before the call runs. The kit's MCPTools wraps this, and is what checks each
// result.

import (
	"context"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"exchange-online-pp-cli/internal/cliutil"
)

func holdToolArguments(next server.ToolHandlerFunc) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		cliutil.HoldCredentialValues(req.GetArguments())
		return next(ctx, req)
	}
}
