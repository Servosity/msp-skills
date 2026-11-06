// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package main

// Hand-fix (handfixes.json credkit-wiring): the kit holds the MCP arguments
// Declarations names. A call's params and body also carry credential members at any
// depth and in any spelling (a desk_phone_pin_code inside a nested body, a JSON body
// passed as a string), so every argument named like a credential is held by the one
// shared walker before the call runs. The kit's MCPTools wraps this and checks each
// result.

import (
	"context"
	"encoding/json"
	"strings"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"zoom-phone-pp-cli/internal/credkit"
)

func holdToolArguments(next server.ToolHandlerFunc) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		args := req.GetArguments()
		if b, err := json.Marshal(args); err == nil {
			credkit.HoldBody(b)
		}
		holdDocuments(args)
		return next(ctx, req)
	}
}

// holdDocuments holds what a JSON document passed as a string carries: a tool's
// body or params can arrive that way, and to HoldBody the outer call is one string.
func holdDocuments(v any) {
	switch t := v.(type) {
	case map[string]any:
		for _, c := range t {
			holdDocuments(c)
		}
	case []any:
		for _, c := range t {
			holdDocuments(c)
		}
	case string:
		if s := strings.TrimSpace(t); strings.HasPrefix(s, "{") || strings.HasPrefix(s, "[") {
			if json.Valid([]byte(s)) {
				credkit.HoldBody([]byte(s))
			}
		}
	}
}
