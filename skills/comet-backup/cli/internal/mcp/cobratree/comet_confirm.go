// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cobratree

// comet-backup handfix (mcp-mirror-write-confirm): a mirrored command that can change
// Comet Server or device state (import, rerun-failed, and any command added
// later) previews, and sends only with confirm set to boolean true, the
// contract comet-backup_execute has. Fail closed: only read-only tools run
// without confirm. A command annotated mcp:local-write, or one in
// mirrorLocalOnlyCommands, writes only this machine's files: its preview runs
// the mirrored command with --help, so the arguments pass the same checks and
// the CLI's parser, the command itself does not run, and the reply echoes them.
//
// Every preview runs the mirrored command through the mirror's own argument
// checks (a blocked or unknown argument is refused by name), sending nothing:
//   - a command annotated mcp:read-only-unless-flags runs as mirrored while no such flag is among the arguments; with
//     one, its preview is the same call without those flags (the plan), and
//     confirm: true runs it as given;
//   - a command annotated comet:sends-with-yes previews by itself until
//     --yes: its preview is the command as mirrored, and confirm: true runs the
//     same arguments with --yes added by this gate (--yes stays a blocked tool
//     argument);
//   - any other command previews with the root --dry-run (the client then
//     sends nothing), and confirm: true runs it as mirrored.
// A tool whose command is not in the tree echoes its arguments instead.

import (
	"context"
	"maps"
	"sort"
	"strings"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"

	"comet-backup-pp-cli/internal/mcp/bound"
)

// SendsWithYesAnnotation marks a command that previews until --yes.
const SendsWithYesAnnotation = "comet:sends-with-yes"

// mirrorLocalOnlyCommands change no Comet state: they never reach the API
// (learnings forget/reject) or only read it (export, sync, workflow archive,
// when a print has them); their writes land in the CLI's own files, which is
// still a write that needs confirm.
var mirrorLocalOnlyCommands = map[string]bool{
	"export":           true,
	"sync":             true,
	"workflow archive": true,
	"learnings forget": true,
	"learnings reject": true,
}

const confirmNote = "Preview only: nothing was sent. Call again with the same arguments and confirm set to boolean true to send it."

// RequireConfirmForMirroredWrites wraps every mirrored tool that may change
// Comet state in a confirm gate. Call it after RegisterAll.
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
		cmd, _, err := root.Find(strings.Fields(path))
		found := err == nil && cmd != root && cmd.CommandPath() == root.Name()+" "+path
		var preview, send server.ToolHandlerFunc
		send = entry.Handler
		var run func(extra string) server.ToolHandlerFunc
		if found {
			blocked := blockedStructuredArgsForCommand(cmd)
			positionals := positionalArgsForCommand(cmd, blocked)
			allowed := allowedStructuredArgsForCommand(cmd, blocked, positionals, commandTakesArgs(cmd))
			run = func(extra string) server.ToolHandlerFunc {
				return guardMirroredMCPCall(cmd, shellOutToCLI(cliPath, append(strings.Fields(path), extra),
					cliFlagBlockedArgs(blocked, positionals), allowed, positionals, false, positionalWriteSinkIndexes(cmd)))
			}
		}
		if mirrorLocalOnlyCommands[path] || (found && isMCPLocalWrite(cmd)) {
			// A local write is a write. --dry-run would not do for its preview:
			// a local command need not honor it, and a sync would still open the
			// writable store. --help stops the command after flags parse.
			// A command parsing its own flags could run on --help: it echoes instead.
			if found && !cmd.DisableFlagParsing {
				preview = helpPreview(name, path, run("--help"))
			}
			addConfirmProperty(&tool, "It writes this machine's local files and sends no change to your Comet Server. Any other value, including the string \"true\", returns a preview.")
			s.AddTool(tool, mirrorConfirmGate(name, path, preview, entry.Handler))
			continue
		}
		if found {
			if unless := readOnlyUnlessFlagNames(cmd); len(unless) > 0 {
				addConfirmProperty(&tool, "It is needed only with "+flagNames(unless)+", which can change state on your Comet Server; without confirm such a call returns the plan the command prints without "+flagNames(unless)+" and sends nothing. Any value other than boolean true, including the string \"true\", previews.")
				s.AddTool(tool, unlessFlagsConfirmGate(name, path, unless, entry.Handler))
				continue
			}
			if annotationIsTrue(cmd, SendsWithYesAnnotation) {
				preview, send = entry.Handler, run("--"+confirmationBypassYesFlag)
			} else {
				preview = withoutArgument("dry-run", run("--dry-run"))
			}
		}
		addConfirmProperty(&tool, "It can change state on your Comet Server or its devices. Any other value, including the string \"true\", returns a preview and sends nothing.")
		s.AddTool(tool, mirrorConfirmGate(name, path, preview, send))
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

func flagNames(set map[string]bool) string {
	names := make([]string, 0, len(set))
	for n := range set {
		names = append(names, "--"+n)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
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
		return echoPreview(name, path, rest), nil
	}
}

func echoPreview(name, path string, args map[string]any) *mcplib.CallToolResult {
	text, err := bound.JSON(map[string]any{
		"preview":               true,
		"confirmation_required": true,
		"tool":                  name,
		"command":               path,
		"arguments":             args,
		"note":                  confirmNote,
	})
	if err != nil {
		return boundedToolResultError("encoding confirmation preview: " + err.Error())
	}
	return mcplib.NewToolResultText(text)
}

// withoutArgument drops a caller's value for the flag the preview itself sets: the
// last flag on a command line wins, and dry-run "false" would send it.
func withoutArgument(flag string, next server.ToolHandlerFunc) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		rest := maps.Clone(req.GetArguments())
		delete(rest, flag)
		req.Params.Arguments = rest
		return next(ctx, req)
	}
}

// helpPreview keeps every refusal check throws, and answers a clean --help with the
// echo: the help page is not the preview the tool promises.
func helpPreview(name, path string, check server.ToolHandlerFunc) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		res, err := check(ctx, req)
		if err != nil || res == nil || !res.IsError || len(res.Content) == 0 {
			return res, err
		}
		if first, ok := res.Content[0].(mcplib.TextContent); !ok || first.Text != cliHelpOnlyResult {
			return res, nil
		}
		return echoPreview(name, path, req.GetArguments()), nil
	}
}

// unlessFlagsConfirmGate runs a command annotated mcp:read-only-unless-flags
// as mirrored while none of those flags is among the arguments (it is a read
// then). With one present, whatever its value, the call previews as the same
// arguments without those flags, and sends only with confirm boolean true.
func unlessFlagsConfirmGate(name, path string, unless map[string]bool, next server.ToolHandlerFunc) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		args := req.GetArguments()
		rest := make(map[string]any, len(args))
		writes := false
		for k, v := range args {
			if k == "confirm" {
				continue
			}
			rest[k] = v
			writes = writes || unless[k]
		}
		if confirmed, ok := args["confirm"].(bool); !writes || (ok && confirmed) {
			req.Params.Arguments = rest
			return next(ctx, req)
		}
		plan := make(map[string]any, len(rest))
		for k, v := range rest {
			if !unless[k] {
				plan[k] = v
			}
		}
		req.Params.Arguments = plan
		res, err := next(ctx, req)
		if res != nil {
			res.Content = append(res.Content, mcplib.NewTextContent(confirmNote))
		}
		return res, err
	}
}
