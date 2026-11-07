// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package mcp

// blumira handfix (mcp-endpoint-write-confirm): every typed endpoint tool that is
// not read-only (assigning owners, commenting on or resolving a finding
// in the org or an MSP sub-account) returns a preview of the request and sends nothing until
// confirm is boolean true.
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

	"blumira-pp-cli/internal/cli"
	"blumira-pp-cli/internal/mcp/bound"
)

const endpointConfirmNote = "Preview only: nothing was sent to Blumira. Call again with the same arguments and confirm set to boolean true to send it."

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
		if recipeTargetIsReadOnly(root, name) {
			// A recipe intent over a read-only command is a read: mark it so
			// instead of gating it (the read-only switch then keeps it too).
			ro, destructive := true, false
			tool.Annotations.ReadOnlyHint, tool.Annotations.DestructiveHint = &ro, &destructive
			s.AddTool(tool, entry.Handler)
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
			"description": "Set to boolean true to send this request. It changes data in your Blumira account. Any other value, including the string \"true\", returns a preview and sends nothing.",
		}
		tool.Description = strings.TrimSpace(tool.Description + " Returns a preview and sends nothing until confirm is boolean true.")
		s.AddTool(tool, endpointConfirmGate(name, method, path, entry.Handler))
	}
}

// recipeReadTargets maps each recipe intent tool (intents.go) to the command it
// runs. The render registers intents with no read-only hint whatever they
// wrap; one is treated as a read only when that command is annotated
// mcp:read-only "true" in the live tree, so a recipe over a write stays gated.
var recipeReadTargets = map[string][]string{
	"find_unprotected_domain_controllers": {"exposure"},
}

func recipeTargetIsReadOnly(root *cobra.Command, tool string) bool {
	path, ok := recipeReadTargets[tool]
	if !ok || root == nil {
		return false
	}
	cmd, _, err := root.Find(path)
	return err == nil && cmd != nil && cmd != root && cmd.Annotations["mcp:read-only"] == "true"
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
