// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cobratree

// prtg handfix (mcp-mirror-write-confirm): a mirrored command that can change
// PRTG state (fleet ack, fleet pause, fleet resume, import, and any command
// added later) previews, and sends only with confirm set to boolean true, the
// contract prtg_execute has. Fail closed: only read-only tools, commands
// annotated mcp:local-write, and the GET-only commands in
// mirrorLocalOnlyCommands run without confirm.
//
// Every preview runs the mirrored command through the mirror's own argument
// checks (a blocked or unknown argument is refused by name), sending nothing:
//   - a command annotated prtg:sends-with-yes previews by itself until --yes
//     (the fleet actions list the matches per core, GETs only): its preview
//     is the command as mirrored, and confirm: true runs the same arguments
//     with --yes added by this gate (--yes stays a blocked tool argument);
//   - any other command previews with the root --dry-run (the client then
//     sends nothing), and confirm: true runs it as mirrored.
// A tool whose command is not in the tree echoes its arguments instead.

import (
	"context"
	"strings"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"

	"prtg-pp-cli/internal/mcp/bound"
)

// SendsWithYesAnnotation marks a command that previews until --yes.
const SendsWithYesAnnotation = "prtg:sends-with-yes"

// mirrorLocalOnlyCommands change no PRTG state: they send only GETs (export,
// sync, fleet sync, workflow archive) or never reach a core (learnings
// forget/reject, jobs prune); their writes land in the CLI's own files.
var mirrorLocalOnlyCommands = map[string]bool{
	"export":           true,
	"sync":             true,
	"fleet sync":       true,
	"workflow archive": true,
	"learnings forget": true,
	"learnings reject": true,
	"jobs prune":       true,
}

const confirmNote = "Preview only: nothing was sent. Call again with the same arguments and confirm set to boolean true to send it."

// RequireConfirmForMirroredWrites wraps every mirrored tool that may change
// PRTG state in a confirm gate. Call it after RegisterAll.
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
		send = entry.Handler
		if found {
			blocked := blockedStructuredArgsForCommand(cmd)
			positionals := positionalArgsForCommand(cmd, blocked)
			allowed := allowedStructuredArgsForCommand(cmd, blocked, positionals, commandTakesArgs(cmd))
			run := func(extra string) server.ToolHandlerFunc {
				return guardMirroredMCPCall(cmd, shellOutToCLI(cliPath, append(strings.Fields(path), extra),
					cliFlagBlockedArgs(blocked, positionals), allowed, positionals, false, positionalWriteSinkIndexes(cmd)))
			}
			if annotationIsTrue(cmd, SendsWithYesAnnotation) {
				preview, send = entry.Handler, run("--"+confirmationBypassYesFlag)
			} else {
				preview = run("--dry-run")
			}
		}
		if tool.InputSchema.Properties == nil {
			tool.InputSchema.Properties = map[string]any{}
		}
		tool.InputSchema.Properties["confirm"] = map[string]any{
			"type":        "boolean",
			"description": "Set to boolean true to send this command. It can change state on your PRTG cores. Any other value, including the string \"true\", returns a preview and sends nothing.",
		}
		tool.Description = strings.TrimSpace(tool.Description + " Returns a preview and sends nothing until confirm is boolean true.")
		s.AddTool(tool, mirrorConfirmGate(name, path, preview, send))
	}
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
		req.Params.Arguments = rest
		if confirmed, ok := args["confirm"].(bool); ok && confirmed {
			return send(ctx, req)
		}
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
