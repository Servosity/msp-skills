// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cobratree

// datto-saas handfix (mcp-mirror-write-confirm): a mirrored command that writes
// (sync, export, teach, learnings, playbook amend, workflow archive and any
// command added later) previews and runs only with confirm set to boolean true,
// the same contract datto-saas_execute has. Fail closed: only read-only tools run
// without confirm, so a command that writes only the CLI's own local files
// previews too.
//
// The gate refuses, by name, any argument the tool does not declare (a host-file
// flag such as --config or --receipt-file is removed from the schema) before it
// previews or runs. A preview starts the child marked DATTO_SAAS_MCP_CHILD=preview: it
// parses every ordinary flag and exits before it touches anything, where a
// command's own --dry-run could still open the local store. Boolean true becomes a bare
// --flag, which a flag that takes a value would read the next token into, so
// boolean true is refused for any flag that takes a value.

import (
	"context"
	"fmt"
	"strings"

	"datto-saas-pp-cli/internal/mcp/bound"
	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"
)

// RequireConfirmForMirroredWrites wraps every mirrored tool that may write in a
// confirm gate. Call it after RegisterAll.
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
		cmd, _, err := root.Find(strings.Fields(path))
		if err != nil || cmd == root || cmd.CommandPath() != root.Name()+" "+path {
			cmd = nil
		}
		if tool.InputSchema.Properties == nil {
			tool.InputSchema.Properties = map[string]any{}
		}
		declared := make(map[string]bool, len(tool.InputSchema.Properties))
		for k := range tool.InputSchema.Properties {
			declared[k] = true
		}
		takesValue := map[string]bool{}
		if cmd != nil {
			for k := range declared {
				if f := cmd.Flag(k); f != nil && f.NoOptDefVal == "" {
					takesValue[k] = true
				}
			}
		}
		tool.InputSchema.Properties["confirm"] = map[string]any{
			"type":        "boolean",
			"description": "Set to boolean true to run this command. It can change local files on this machine. Any other value, including the string \"true\", returns a preview and runs nothing.",
		}
		tool.Description = strings.TrimSpace(tool.Description + " Returns a preview and runs nothing until confirm is boolean true.")
		s.AddTool(tool, mirrorConfirmGate(name, path, declared, takesValue, entry.Handler))
	}
}

func mirrorConfirmGate(name, path string, declared, takesValue map[string]bool, next server.ToolHandlerFunc) server.ToolHandlerFunc {
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
		for k, v := range rest {
			if v == true && takesValue[k] {
				return boundedToolResultError(fmt.Sprintf("parameter %q takes a value; boolean true is not allowed", k)), nil
			}
		}
		if confirmed, ok := args["confirm"].(bool); ok && confirmed {
			req.Params.Arguments = rest
			return next(ctx, req)
		}
		req.Params.Arguments = rest
		res, err := next(context.WithValue(ctx, previewKey{}, true), req)
		if err != nil || res == nil || res.IsError {
			return res, err
		}
		text, err := bound.JSON(map[string]any{
			"preview":               true,
			"confirmation_required": true,
			"tool":                  name,
			"command":               path,
			"arguments":             rest,
			"note":                  "Nothing was run; the flags and arguments were parsed. Call again with the same arguments and confirm set to boolean true.",
		})
		if err != nil {
			return boundedToolResultError("encoding confirmation preview: " + err.Error()), nil
		}
		return mcplib.NewToolResultText(text), nil
	}
}

type previewKey struct{}

// previewRun reports a child started for a preview, not to do the work.
func previewRun(ctx context.Context) bool {
	v, _ := ctx.Value(previewKey{}).(bool)
	return v
}
