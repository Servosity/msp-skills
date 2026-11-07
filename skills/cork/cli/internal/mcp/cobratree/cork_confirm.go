// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cobratree

// cork handfix (mcp-mirror-write-confirm, fleet precedent: skykick, gradient):
// a mirrored command that can change Cork data or the operator's machine
// (import, and any command a later print adds) previews, and sends only with
// confirm set to boolean true. Fail closed: only read-only tools, commands
// annotated mcp:local-write and the local commands in mirrorLocalOnlyCommands
// run without confirm. A gated command previews with the root --dry-run (the
// client then sends nothing), and confirm: true runs it as mirrored.
// mirrorSendFlags is empty on cork (no command sends behind its own boolean
// flag) and stays as the hook for one that does. A tool whose command is not
// in the tree echoes its arguments instead.

import (
	"context"
	"strings"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"

	"cork-pp-cli/internal/mcp/bound"
)

// mirrorSendFlags names the boolean flag that turns a command's preview into
// a send.
var mirrorSendFlags = map[string]string{}

// mirrorLocalOnlyCommands change no Cork data: they only read (export) or
// write the CLI's own files (sync, workflow archive, the learn store).
var mirrorLocalOnlyCommands = map[string]bool{
	"export":            true,
	"sync":              true,
	"workflow archive":  true,
	"learnings confirm": true,
	"learnings forget":  true,
	"learnings reject":  true,
	"teach lookup":      true,
	"teach pattern":     true,
	"playbook amend":    true,
}

const confirmNote = "Preview only: nothing was sent. Call again with the same arguments and confirm set to boolean true to send it."

// RequireConfirmForMirroredWrites wraps every mirrored tool that may change
// Cork data in a confirm gate. Call it after RegisterAll.
func RequireConfirmForMirroredWrites(s *server.MCPServer, root *cobra.Command, cliPath func() (string, error)) {
	cliPath = shareCLIPath(cliPath)
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
		found := err == nil && cmd != root && cmd.CommandPath() == root.Name()+" "+path
		if found && isMCPLocalWrite(cmd) {
			continue
		}
		var preview, send server.ToolHandlerFunc
		// owned: the arguments the gate sets itself. A caller value for one
		// (apply true, dry-run 0) would turn the preview into a send, so the
		// preview drops them, whatever their type or value.
		var owned []string
		send = entry.Handler
		if found {
			blocked := blockedStructuredArgsForCommand(cmd)
			positionals := positionalArgsForCommand(cmd, blocked)
			allowed := allowedStructuredArgsForCommand(cmd, blocked, positionals, commandTakesArgs(cmd))
			run := func(extra string) server.ToolHandlerFunc {
				return guardMirroredMCPCall(cmd, shellOutToCLI(cliPath, append(strings.Fields(path), extra),
					cliFlagBlockedArgs(blocked, positionals), allowed, positionals, false, positionalWriteSinkIndexes(cmd)))
			}
			if sf := mirrorSendFlags[path]; sf != "" && boolFlag(cmd, sf) {
				preview, send, owned = entry.Handler, run("--"+sf), []string{sf}
				delete(tool.InputSchema.Properties, sf)
			} else {
				preview, owned = run("--dry-run"), []string{"dry-run"}
			}
		}
		addConfirmProperty(&tool, "It can change data in your Cork account. Any other value, including the string \"true\", returns a preview and sends nothing.")
		s.AddTool(tool, mirrorConfirmGate(name, path, preview, send, owned))
	}
}

func addConfirmProperty(tool *mcplib.Tool, why string) {
	if tool.InputSchema.Properties == nil {
		tool.InputSchema.Properties = map[string]any{}
	}
	tool.InputSchema.Properties["confirm"] = map[string]any{
		"type":        "boolean",
		"description": "Set to boolean true to send this command. " + why,
	}
	tool.Description = strings.TrimSpace(tool.Description + " Returns a preview and sends nothing until confirm is boolean true.")
}

func mirrorConfirmGate(name, path string, preview, send server.ToolHandlerFunc, owned []string) server.ToolHandlerFunc {
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
		for _, k := range owned {
			delete(rest, k)
		}
		req.Params.Arguments = rest
		if preview != nil {
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

// boolFlag reports a boolean flag the command itself declares; anything else
// falls back to the --dry-run preview (fail closed).
func boolFlag(cmd *cobra.Command, name string) bool {
	f := cmd.LocalFlags().Lookup(name)
	return f != nil && f.Value.Type() == "bool"
}
