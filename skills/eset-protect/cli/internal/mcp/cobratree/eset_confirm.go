// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cobratree

// mcp-mirror-write-confirm: a mirrored command that is not read-only previews and
// runs only with confirm set to boolean true, the same contract
// eset-protect_execute has. Fail closed: only tools marked read-only run without
// it, so the local-write commands (learnings forget --all deletes rows) and any
// command added later are gated too.
//
// The gate refuses, by name, any argument the tool does not declare (a host-file
// flag such as --config is not in the schema), and whatever the call-time guards
// refuse, before it previews or runs. Boolean true becomes a bare --flag, which a
// flag that takes a value would read the next token into, so boolean true is refused
// for any flag that takes a value.
//
// A preview starts the CLI in parse-only mode (cliutil.MCPChildEnvVar set to
// cliutil.MCPChildPreview, only through the context value withMCPParseOnlyPreview;
// every other child gets "1"): the CLI parses the same command
// line a confirmed call would get, reports a flag or value it cannot parse, prints one
// line naming what it parsed, and runs nothing and writes nothing (internal/cli
// eset_mcp_preview.go). A command's --dry-run is not used: it is not proven read-only
// (sync opens and migrates its store before it checks --dry-run). The child
// environment always drops every inherited child marker, so only this path sets the
// preview value and a confirmed call cannot be neutered. A tool whose command the tree cannot name
// previews without starting anything.

import (
	"context"
	"fmt"
	"strings"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"

	"eset-protect-pp-cli/internal/mcp/bound"
)

// RequireConfirmForMirroredWrites wraps every mirrored tool that is not read-only
// in a confirm gate. Call it after RegisterAll.
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
			"description": "Set to boolean true to run this command. It can change your ESET tenant or this CLI's local data. Any other value, including the string \"true\", returns a preview and runs nothing.",
		}
		tool.Description = strings.TrimSpace(tool.Description + " Returns a preview and runs nothing until confirm is boolean true.")
		s.AddTool(tool, mirrorConfirmGate(name, path, cmd, declared, takesValue, entry.Handler))
	}
}

func mirrorConfirmGate(name, path string, cmd *cobra.Command, declared, takesValue map[string]bool, next server.ToolHandlerFunc) server.ToolHandlerFunc {
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
		if cmd != nil {
			if msg := esetMirroredCallRefusal(cmd, rest); msg != "" {
				return boundedToolResultError(msg), nil
			}
			if msg := mirroredMCPReadOnlyRefusal(cmd, rest); msg != "" {
				return boundedToolResultError(msg), nil
			}
		}
		req.Params.Arguments = rest
		if confirmed, ok := args["confirm"].(bool); ok && confirmed {
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
		if cmd != nil {
			res, err := next(withMCPParseOnlyPreview(ctx), req)
			if err != nil || res == nil || res.IsError {
				return res, err
			}
			var out string
			if len(res.Content) > 0 {
				if tc, ok := res.Content[0].(mcplib.TextContent); ok {
					out = strings.TrimSpace(tc.Text)
				}
			}
			preview["parse_only"] = out
			preview["note"] = "The CLI only parsed these arguments (parse_only); nothing was run or written. Call again with the same arguments and confirm set to boolean true to run it."
		}
		text, err := bound.JSON(preview)
		if err != nil {
			return boundedToolResultError("encoding confirmation preview: " + err.Error()), nil
		}
		return mcplib.NewToolResultText(text), nil
	}
}

// mcpParseOnlyPreviewKey marks a context whose CLI child runs in parse-only mode.
type mcpParseOnlyPreviewKey struct{}

// withMCPParseOnlyPreview asks RunCLICommand to start the child with the child
// marker set to cliutil.MCPChildPreview. Only the confirm gate's preview calls it.
func withMCPParseOnlyPreview(ctx context.Context) context.Context {
	return context.WithValue(ctx, mcpParseOnlyPreviewKey{}, true)
}

func mcpParseOnlyPreviewRequested(ctx context.Context) bool {
	v, _ := ctx.Value(mcpParseOnlyPreviewKey{}).(bool)
	return v
}
