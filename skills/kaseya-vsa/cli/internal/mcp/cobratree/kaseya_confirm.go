// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cobratree

// kaseya-vsa handfix (mcp-mirror-write-confirm): a mirrored command that can
// change VSA state (scripts run-wait, tasks run-wait, import, and any command
// added later) previews and runs only with confirm set to boolean true, the
// same contract kaseya-vsa_execute has. Fail closed: only read-only tools,
// commands annotated mcp:local-write, and the commands listed in
// mirrorLocalOnlyCommands run without confirm.
//
// The gate refuses, by name, any argument the tool does not declare (a
// host-file flag such as --config or --receipt-file is removed from the
// schema) before it previews or runs. The preview runs the command with the
// root --dry-run, so every ordinary flag reaches the CLI and the VSA client
// sends nothing; a command whose --dry-run is not the root flag gets a
// preview that runs nothing.

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
		cmd, _, err := root.Find(strings.Fields(path))
		if err != nil || cmd == root || cmd.CommandPath() != root.Name()+" "+path {
			cmd = nil
		}
		if cmd != nil && isMCPLocalWrite(cmd) {
			continue
		}
		if tool.InputSchema.Properties == nil {
			tool.InputSchema.Properties = map[string]any{}
		}
		declared := make(map[string]bool, len(tool.InputSchema.Properties))
		for k := range tool.InputSchema.Properties {
			declared[k] = true
		}
		rootDryRun := root.PersistentFlags().Lookup("dry-run")
		dryRun := declared["dry-run"] && rootDryRun != nil && cmd != nil && cmd.Flag("dry-run") == rootDryRun
		tool.InputSchema.Properties["confirm"] = map[string]any{
			"type":        "boolean",
			"description": "Set to boolean true to run this command. It can change state in your VSA tenant. Any other value, including the string \"true\", returns a preview and runs nothing.",
		}
		tool.Description = strings.TrimSpace(tool.Description + " Returns a preview and runs nothing until confirm is boolean true.")
		s.AddTool(tool, mirrorConfirmGate(name, path, declared, dryRun, entry.Handler))
	}
}

func mirrorConfirmGate(name, path string, declared map[string]bool, dryRun bool, next server.ToolHandlerFunc) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		args := req.GetArguments()
		rest := make(map[string]any, len(args))
		for k, v := range args {
			if k != "confirm" {
				rest[k] = v
			}
		}
		if err := validateMCPArgumentNames(rest, declared); err != nil {
			return boundedToolResultError(err.Error()), nil
		}
		if confirmed, ok := args["confirm"].(bool); ok && confirmed {
			req.Params.Arguments = rest
			return next(ctx, req)
		}
		preview := map[string]any{
			"preview":               true,
			"confirmation_required": true,
			"tool":                  name,
			"command":               path,
			"arguments":             rest,
			"note":                  "Nothing was run. Call again with the same arguments and confirm set to boolean true.",
		}
		if dryRun {
			dry := make(map[string]any, len(rest)+1)
			for k, v := range rest {
				dry[k] = v
			}
			dry["dry-run"] = true
			req.Params.Arguments = dry
			res, err := next(ctx, req)
			if err != nil || res == nil || res.IsError {
				return res, err
			}
			var out strings.Builder
			for _, c := range res.Content {
				if tc, ok := c.(mcplib.TextContent); ok {
					out.WriteString(tc.Text)
				}
			}
			preview["dry_run"] = out.String()
			preview["note"] = "Nothing was sent to VSA; dry_run is the command's own --dry-run output. Call again with the same arguments and confirm set to boolean true to run it."
		}
		text, err := bound.JSON(preview)
		if err != nil {
			return boundedToolResultError("encoding confirmation preview: " + err.Error()), nil
		}
		return mcplib.NewToolResultText(text), nil
	}
}
