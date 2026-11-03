// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written (NOT generated): pinned by handfixes.json under "mcp-write-confirm".

package cobratree

// A mirrored command that is not read-only previews, and runs only with
// confirm set to boolean true. Fail closed: there is no local-only exemption,
// because a write to the CLI's own files (store, learnings, archive) is still a
// write the caller did not confirm.
//
// The preview runs the mirrored command with the root --dry-run through the
// mirror's own argument checks, so a host-file flag is refused by name and an
// ordinary flag still reaches the CLI; the CLI's dry-run sends nothing and
// writes nothing. A caller "dry-run" argument is dropped from the preview,
// since on the child's command line it would follow the injected flag and
// turn it off. A tool whose command is not in the tree echoes its arguments.

import (
	"context"
	"strings"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"

	"xero-pp-cli/internal/mcp/bound"
)

const confirmNote = "Preview only: nothing was sent. Call again with the same arguments and confirm set to boolean true to send it."

// RequireConfirmForMirroredWrites wraps every mirrored tool that is not
// read-only in a confirm gate. Call it after RegisterAll.
func RequireConfirmForMirroredWrites(s *server.MCPServer, root *cobra.Command, cliPath func() (string, error)) {
	cliPath = shareCLIPath(cliPath)
	for name, entry := range s.ListTools() {
		tool := entry.Tool
		if !IsMirroredTool(tool) {
			continue
		}
		if hint := tool.Annotations.ReadOnlyHint; hint != nil && *hint {
			continue
		}
		path, _ := tool.Meta.AdditionalFields[mirrorCLICommandMetaKey].(string)
		var preview server.ToolHandlerFunc
		cmd, _, err := root.Find(strings.Fields(path))
		if err == nil && cmd != root && cmd.CommandPath() == root.Name()+" "+path {
			blocked := blockedStructuredArgsForCommand(cmd)
			positionals := positionalArgsForCommand(cmd, blocked)
			allowed := allowedStructuredArgsForCommand(cmd, blocked, positionals, commandTakesArgs(cmd))
			preview = guardMirroredMCPCall(cmd, shellOutToCLI(cliPath, append(strings.Fields(path), "--dry-run"),
				cliFlagBlockedArgs(blocked, positionals), allowed, positionals, false, positionalWriteSinkIndexes(cmd)))
		}
		if tool.InputSchema.Properties == nil {
			tool.InputSchema.Properties = map[string]any{}
		}
		tool.InputSchema.Properties["confirm"] = map[string]any{
			"type":        "boolean",
			"description": "Set to boolean true to run this command. It can change data in your Xero organisation or this machine's local files. Any other value, including the string \"true\", returns a preview and changes nothing.",
		}
		tool.Description = strings.TrimSpace(tool.Description + " Returns a preview and changes nothing until confirm is boolean true.")
		s.AddTool(tool, mirrorConfirmGate(name, path, preview, entry.Handler))
	}
}

// IsMirroredTool reports a tool the cobratree mirror registered (it runs the
// companion CLI in a child process).
func IsMirroredTool(tool mcplib.Tool) bool {
	return tool.Meta != nil && tool.Meta.AdditionalFields["pp:tenant-gate"] == "child-cli"
}

func mirrorConfirmGate(name, path string, preview, send server.ToolHandlerFunc) server.ToolHandlerFunc {
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
		if preview != nil {
			plan := make(map[string]any, len(rest))
			for k, v := range rest {
				if k != "dry-run" {
					plan[k] = v
				}
			}
			req.Params.Arguments = plan
			res, err := preview(ctx, req)
			if res != nil {
				res.Content = append(res.Content, mcplib.NewTextContent(confirmNote))
			}
			return res, err
		}
		text, err := bound.JSON(map[string]any{
			"preview":               true,
			"confirmation_required": true,
			"tool":                  name,
			"command":               path,
			"arguments":             rest,
			"note":                  confirmNote,
		})
		if err != nil {
			return boundedToolResultError("encoding confirmation preview: " + err.Error()), nil
		}
		return mcplib.NewToolResultText(text), nil
	}
}
