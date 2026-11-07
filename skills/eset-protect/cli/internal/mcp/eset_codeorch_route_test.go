// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package mcp

import (
	"context"
	"strings"

	mcplib "github.com/mark3labs/mcp-go/mcp"
)

// codeOrchCall calls an endpoint the way a host does under the 4.34.2
// read/execute split: eset-protect_read first, and an endpoint the read tool
// refuses as state-changing goes through eset-protect_execute with confirm
// set to boolean true. The hand-fix tests written against the single
// execute tool route through it.
func codeOrchCall(ctx context.Context, args map[string]any) (*mcplib.CallToolResult, error) {
	res, err := handleCodeOrchRead(ctx, mcplib.CallToolRequest{Params: mcplib.CallToolParams{Arguments: args}})
	if err != nil || res == nil || !res.IsError || !strings.Contains(codeOrchResultText(res), "call eset-protect_execute") {
		return res, err
	}
	confirmed := make(map[string]any, len(args)+1)
	for k, v := range args {
		confirmed[k] = v
	}
	confirmed["confirm"] = true
	return handleCodeOrchExecute(ctx, mcplib.CallToolRequest{Params: mcplib.CallToolParams{Arguments: confirmed}})
}

func codeOrchResultText(res *mcplib.CallToolResult) string {
	var b strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(mcplib.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String()
}
