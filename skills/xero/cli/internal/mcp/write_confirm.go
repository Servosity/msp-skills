// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written (NOT generated): pinned by handfixes.json under "mcp-write-confirm".

package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"xero-pp-cli/internal/mcp/bound"
)

// confirmArg is the tool argument that turns a write preview into a send.
const confirmArg = "confirm"

// requireConfirmOnWrites makes every MCP tool that is not annotated
// readOnlyHint=true preview what it would do and send nothing until the
// caller passes confirm as boolean true. Reads are the allowlist; anything
// else, including a tool registered by a future print with no annotation,
// fails closed. It is the same predicate the read-only switch
// (dropNonReadOnlyTools) uses, so the two can never disagree about which
// tools write. It must run after every registration.
func requireConfirmOnWrites(s *server.MCPServer) {
	for name, entry := range s.ListTools() {
		if hint := entry.Tool.Annotations.ReadOnlyHint; hint != nil && *hint {
			continue
		}
		tool, err := withConfirmParam(entry.Tool)
		if err != nil {
			// A schema this gate cannot extend is a tool that would otherwise
			// write without confirm: remove it rather than ship it ungated.
			s.DeleteTools(name)
			continue
		}
		s.AddTool(tool, confirmGate(name, entry.Handler))
	}
}

func confirmParamSchema() map[string]any {
	return map[string]any{
		"type":        "boolean",
		"description": "Set to boolean true to perform this call. Any other value, including the string \"true\", returns a preview and does not contact the API or change local state.",
	}
}

// withConfirmParam adds the confirm property to the tool's input schema
// without sharing maps with the registered copy.
func withConfirmParam(tool mcplib.Tool) (mcplib.Tool, error) {
	if len(tool.RawInputSchema) > 0 {
		var schema map[string]any
		if err := json.Unmarshal(tool.RawInputSchema, &schema); err != nil {
			return tool, err
		}
		props, _ := schema["properties"].(map[string]any)
		next := make(map[string]any, len(props)+1)
		for k, v := range props {
			next[k] = v
		}
		next[confirmArg] = confirmParamSchema()
		schema["properties"] = next
		raw, err := json.Marshal(schema)
		if err != nil {
			return tool, err
		}
		tool.RawInputSchema = raw
		return tool, nil
	}
	next := make(map[string]any, len(tool.InputSchema.Properties)+1)
	for k, v := range tool.InputSchema.Properties {
		next[k] = v
	}
	next[confirmArg] = confirmParamSchema()
	tool.InputSchema.Properties = next
	if tool.InputSchema.Type == "" {
		tool.InputSchema.Type = "object"
	}
	return tool, nil
}

// confirmGate strips confirm before the wrapped handler sees the arguments
// (the generated handlers reject undeclared names) and sends only when it is
// boolean true. Otherwise it answers with a preview of the call.
func confirmGate(name string, next server.ToolHandlerFunc) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		args := req.GetArguments()
		forward := make(map[string]any, len(args))
		confirmed := false
		for k, v := range args {
			if k == confirmArg {
				b, ok := v.(bool)
				confirmed = ok && b
				continue
			}
			forward[k] = v
		}
		if !confirmed {
			text, err := bound.JSON(map[string]any{
				"preview":               true,
				"confirmation_required": true,
				"sent":                  false,
				"tool":                  name,
				"arguments":             forward,
				"next_step":             "Nothing was sent. Call this tool again with the same arguments and confirm set to boolean true to perform it.",
			})
			if err != nil {
				return mcplib.NewToolResultError(fmt.Sprintf("encoding confirmation preview: %v", err)), nil
			}
			return mcplib.NewToolResultText(text), nil
		}
		req.Params.Arguments = forward
		return next(ctx, req)
	}
}
