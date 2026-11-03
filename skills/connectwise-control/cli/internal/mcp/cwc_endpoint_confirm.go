// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package mcp

// connectwise-control handfix (mcp-endpoint-write-confirm): every registered tool that is
// not read-only and carries no confirm of its own (a recipe intent a later print
// lifts; connectwise_control_execute has the generated confirm) returns a preview
// of the call and sends nothing until confirm is boolean true.
// Fail closed: the gate wraps every registered tool that lacks the read-only
// hint and is not a mirrored command (those carry their own gate,
// cobratree.RequireConfirmForMirroredWrites), so an endpoint a later print
// adds is gated too.

import (
	"context"
	"strings"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"

	"connectwise-control-pp-cli/internal/cli"
	"connectwise-control-pp-cli/internal/mcp/bound"
)

const endpointConfirmNote = "Preview only: nothing was sent to ConnectWise Control. Call again with the same arguments and confirm set to boolean true to send it."

// RequireConfirmForEndpointWrites wraps every non-read-only, non-mirrored tool
// in the confirm gate. Call it after every tool is registered.
func RequireConfirmForEndpointWrites(s *server.MCPServer) {
	root := cli.RootCmd()
	for name, entry := range s.ListTools() {
		tool := entry.Tool
		if tool.Meta != nil && tool.Meta.AdditionalFields["pp:tenant-gate"] == "child-cli" {
			continue
		}
		if hint := tool.Annotations.ReadOnlyHint; hint != nil && *hint {
			continue
		}
		if _, gated := tool.InputSchema.Properties["confirm"]; gated {
			continue
		}
		method, path := endpointRequest(root, name)
		if tool.InputSchema.Properties == nil {
			tool.InputSchema.Properties = map[string]any{}
		}
		tool.InputSchema.Properties["confirm"] = map[string]any{
			"type":        "boolean",
			"description": "Set to boolean true to send this request. It changes data in your ConnectWise Control account. Any other value, including the string \"true\", returns a preview and sends nothing.",
		}
		tool.Description = strings.TrimSpace(tool.Description + " Returns a preview and sends nothing until confirm is boolean true.")
		s.AddTool(tool, endpointConfirmGate(name, method, path, entry.Handler))
	}
}

// endpointRequest names the HTTP method and path template of the command a
// tool mirrors (tool accounts_update-one is command
// `accounts update-one`), for the preview; "" when unknown.
func endpointRequest(root *cobra.Command, tool string) (string, string) {
	if root == nil {
		return "", ""
	}
	cmd, _, err := root.Find(strings.Split(tool, "_"))
	if err != nil || cmd == nil || cmd == root {
		return "", ""
	}
	return cmd.Annotations["pp:method"], cmd.Annotations["pp:path"]
}

func endpointConfirmGate(name, method, path string, send server.ToolHandlerFunc) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		args := req.GetArguments()
		rest := make(map[string]any, len(args))
		for k, v := range args {
			if k != "confirm" {
				rest[k] = v
			}
		}
		if confirmed, ok := args["confirm"].(bool); ok && confirmed {
			req.Params.Arguments = rest
			return send(ctx, req)
		}
		preview := map[string]any{
			"preview":               true,
			"confirmation_required": true,
			"tool":                  name,
			"arguments":             rest,
			"note":                  endpointConfirmNote,
		}
		if method != "" {
			preview["method"], preview["path"] = method, path
		}
		text, err := bound.JSON(preview)
		if err != nil {
			return mcpToolError("encoding confirmation preview: " + err.Error()), nil
		}
		return mcplib.NewToolResultText(text), nil
	}
}
