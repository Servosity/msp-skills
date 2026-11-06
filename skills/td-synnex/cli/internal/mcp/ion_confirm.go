// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored (handfixes.json: ion-mcp-write-confirm).

package mcp

// Every MCP tool that can act on StreamOne ION previews, and sends only with
// confirm set to boolean true:
//   - an endpoint tool that is not read-only (customer orders create and
//     cancel, cart checkout, cart and cart-item writes, customer create and
//     update, product enable and disable, provisioning, report save and
//     delete) returns a preview of the request and sends nothing until
//     confirm is boolean true;
//   - a typed write with its own --confirm (subscriptions set) previews until
//     that flag; confirm reaches it only when it is boolean true, so the
//     string "true" still previews.
// Fail closed: only read-only tools and the mirrored commands in
// localOnlyMirroredTools (they change the local store, the learn store or
// stdout, never StreamOne ION) run without the gate, so a write a later
// print adds is gated too.

import (
	"context"
	"strings"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"

	"td-synnex-pp-cli/internal/cli"
	"td-synnex-pp-cli/internal/mcp/bound"
)

const endpointConfirmNote = "Preview only: nothing was sent to StreamOne ION. Call again with the same arguments and confirm set to boolean true to send it."

// localOnlyMirroredTools are mirrored commands that are not read-only but
// write only local state: the local store (sync, workflow_archive), stdout
// (export; its --output flag is blocked over MCP), and the learn store
// (learnings, teach, playbook).
var localOnlyMirroredTools = map[string]bool{
	"sync": true, "workflow_archive": true, "export": true,
	"learnings_confirm": true, "learnings_forget": true, "learnings_reject": true,
	"playbook_amend": true, "teach": true, "teach_lookup": true, "teach_pattern": true, "teach_playbook": true,
}

func isMirroredTool(tool mcplib.Tool) bool {
	return tool.Meta != nil && tool.Meta.AdditionalFields["pp:tenant-gate"] == "child-cli"
}

// RequireConfirmForWrites gates every write tool. Call it after every tool is
// registered.
func RequireConfirmForWrites(s *server.MCPServer) {
	root := cli.RootCmd()
	for name, entry := range s.ListTools() {
		tool := entry.Tool
		if hint := tool.Annotations.ReadOnlyHint; hint != nil && *hint {
			continue
		}
		if p, ok := tool.InputSchema.Properties["confirm"].(map[string]any); ok && p["type"] == "boolean" {
			s.AddTool(tool, booleanConfirmOnly(entry.Handler))
			continue
		}
		if localOnlyMirroredTools[name] && isMirroredTool(tool) {
			continue
		}
		method, path := endpointRequest(root, name)
		props := make(map[string]any, len(tool.InputSchema.Properties)+1)
		for k, v := range tool.InputSchema.Properties {
			props[k] = v
		}
		props["confirm"] = map[string]any{
			"type":        "boolean",
			"description": "Set to boolean true to send this request to StreamOne ION. Any other value, including the string \"true\", returns a preview and sends nothing.",
		}
		tool.InputSchema.Properties = props
		tool.Description = strings.TrimSpace(tool.Description + " Returns a preview and sends nothing until confirm is boolean true.")
		s.AddTool(tool, endpointConfirmGate(name, method, path, entry.Handler))
	}
}

// booleanConfirmOnly drops a confirm argument that is not boolean true, so the
// command keeps its preview default.
func booleanConfirmOnly(next server.ToolHandlerFunc) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		args := req.GetArguments()
		if v, ok := args["confirm"]; ok {
			if b, isBool := v.(bool); !isBool || !b {
				rest := make(map[string]any, len(args))
				for k, val := range args {
					if k != "confirm" {
						rest[k] = val
					}
				}
				req.Params.Arguments = rest
			}
		}
		return next(ctx, req)
	}
}

// endpointRequest names the HTTP method and path template of the command a
// tool mirrors (tool carts_checkout is command `carts checkout`); "" when unknown.
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
