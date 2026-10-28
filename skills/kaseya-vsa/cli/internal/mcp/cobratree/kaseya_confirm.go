// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cobratree

// kaseya-vsa handfix (mcp-mirror-write-confirm): a mirrored command that can
// change VSA state (scripts run-wait, tasks run-wait, import, and any command
// added later) previews and runs only with confirm set to boolean true, the
// same contract kaseya-vsa_execute has. Fail closed: only read-only tools,
// commands annotated mcp:local-write, and the commands listed in
// mirrorLocalOnlyCommands run without confirm.

import (
	"context"
	"strings"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"
	"kaseya-vsa-pp-cli/internal/mcp/bound"
)

// mirrorLocalOnlyCommands send only GET requests to VSA; their writes land in
// the CLI's own local store.
var mirrorLocalOnlyCommands = map[string]bool{
	"export":           true,
	"sync":             true,
	"workflow archive": true,
}

// RequireConfirmForMirroredWrites wraps every mirrored tool that may change
// VSA state in a confirm gate. Call it after RegisterAll.
func RequireConfirmForMirroredWrites(s *server.MCPServer, root *cobra.Command) {
	for name, entry := range s.ListTools() {
		tool := entry.Tool
		if tool.Meta == nil || tool.Meta.AdditionalFields["pp:tenant-gate"] != "child-cli" {
			continue
		}
		if hint := tool.Annotations.ReadOnlyHint; hint != nil && *hint {
			continue
		}
		path, _ := tool.Meta.AdditionalFields[mirrorCLICommandMetaKey].(string)
		if mirrorLocalOnlyCommands[path] {
			continue
		}
		if cmd, _, err := root.Find(strings.Fields(path)); err == nil && cmd != root && cmd.CommandPath() == root.Name()+" "+path && isMCPLocalWrite(cmd) {
			continue
		}
		if tool.InputSchema.Properties == nil {
			tool.InputSchema.Properties = map[string]any{}
		}
		tool.InputSchema.Properties["confirm"] = map[string]any{
			"type":        "boolean",
			"description": "Set to boolean true to run this command. It can change state in your VSA tenant. Any other value, including the string \"true\", returns a preview and runs nothing.",
		}
		tool.Description = strings.TrimSpace(tool.Description + " Returns a preview and runs nothing until confirm is boolean true.")
		s.AddTool(tool, mirrorConfirmGate(name, path, entry.Handler))
	}
}

func mirrorConfirmGate(name, path string, next server.ToolHandlerFunc) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		args := req.GetArguments()
		rest := make(map[string]any, len(args))
		for k, v := range args {
			if k != "confirm" {
				rest[k] = v
			}
		}
		if confirmed, ok := args["confirm"].(bool); !ok || !confirmed {
			text, err := bound.JSON(map[string]any{
				"preview":               true,
				"confirmation_required": true,
				"tool":                  name,
				"command":               path,
				"arguments":             rest,
				"note":                  "Nothing was run. Call again with the same arguments and confirm set to boolean true.",
			})
			if err != nil {
				return boundedToolResultError("encoding confirmation preview: " + err.Error()), nil
			}
			return mcplib.NewToolResultText(text), nil
		}
		req.Params.Arguments = rest
		return next(ctx, req)
	}
}
