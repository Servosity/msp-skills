// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cobratree

// umbrella handfix (mcp-mirror-write-confirm): a mirrored command that can change
// Umbrella state (import, destination-lists diff --apply, and any command
// added later) previews, and sends only with confirm set to boolean true, the
// contract cisco-umbrella_execute has. Fail closed: only read-only tools,
// commands annotated mcp:local-write, and the GET-only or local commands in
// mirrorLocalOnlyCommands run without confirm.
//
// Every preview runs the mirrored command through the mirror's own argument
// checks (a blocked or unknown argument is refused by name), sending nothing:
//   - a command annotated mcp:read-only-unless-flags (destination-lists diff:
//     --apply) runs as mirrored while no such flag is among the arguments (it is
//     a read then); with one, it runs nothing and returns the argv (the plan is
//     the same call without that flag), and confirm: true runs it as given;
//   - a command annotated umbrella:sends-with-yes previews by itself until
//     --yes: its preview is the command as mirrored, and confirm: true runs the
//     same arguments with --yes added by this gate (--yes stays a blocked tool
//     argument);
//   - a command in dryRunPreviews previews with the root --dry-run (the client
//     then sends nothing), and confirm: true runs it as mirrored;
//   - any other command runs nothing: its preview refuses an undeclared argument
//     (a host-file flag) by name and returns the argv. A preview must open no
//     writable store and write no file, and nothing has proved that of a command
//     no one reviewed here.
// A tool whose command is not in the tree echoes its arguments instead.

import (
	"context"
	"sort"
	"strings"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"

	"cisco-umbrella-pp-cli/internal/mcp/bound"
)

// SendsWithYesAnnotation marks a command that previews until --yes.
const SendsWithYesAnnotation = "umbrella:sends-with-yes"

// mirrorLocalOnlyCommands change no Umbrella state: they send only GETs
// (export, sync, workflow archive) or never reach the API (learnings
// forget/reject); their writes land in the CLI's own files.
var mirrorLocalOnlyCommands = map[string]bool{
	"export":           true,
	"sync":             true,
	"workflow archive": true,
	"learnings forget": true,
	"learnings reject": true,
}

// dryRunPreviews are the commands whose --dry-run is reviewed to open no writable
// store and write no file (TestMCPPreviewWritesNothing runs each on the built
// binaries): import reads its input and hands each record to a client that sends
// nothing under --dry-run.
var dryRunPreviews = map[string]bool{"import": true}

const confirmNote = "Preview only: nothing was sent. Call again with the same arguments and confirm set to boolean true to send it."

// RequireConfirmForMirroredWrites wraps every mirrored tool that may change
// Umbrella state in a confirm gate. Call it after RegisterAll.
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
		var check func(map[string]any) error
		send = entry.Handler
		if found {
			blocked := blockedStructuredArgsForCommand(cmd)
			positionals := positionalArgsForCommand(cmd, blocked)
			allowed := allowedStructuredArgsForCommand(cmd, blocked, positionals, commandTakesArgs(cmd))
			run := func(extra string) server.ToolHandlerFunc {
				return guardMirroredMCPCall(cmd, shellOutToCLI(cliPath, append(strings.Fields(path), extra),
					cliFlagBlockedArgs(blocked, positionals), allowed, positionals, false, positionalWriteSinkIndexes(cmd)))
			}
			if unless := readOnlyUnlessFlagNames(cmd); len(unless) > 0 {
				addConfirmProperty(&tool, "It is needed only with "+flagNames(unless)+", which can change state in your Umbrella organization; without confirm such a call runs nothing and returns its arguments. Call without "+flagNames(unless)+" for the plan. Any value other than boolean true, including the string \"true\", previews.")
				s.AddTool(tool, unlessFlagsConfirmGate(name, path, unless, allowed, entry.Handler))
				continue
			}
			switch {
			case annotationIsTrue(cmd, SendsWithYesAnnotation):
				preview, send = entry.Handler, run("--"+confirmationBypassYesFlag)
			case dryRunPreviews[path]:
				preview = run("--dry-run")
			default:
				check = func(args map[string]any) error { return validateMCPArgumentNames(args, allowed) }
			}
		}
		addConfirmProperty(&tool, "It can change state in your Umbrella organization. Any other value, including the string \"true\", returns a preview and sends nothing.")
		s.AddTool(tool, mirrorConfirmGate(name, path, check, preview, send))
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

func mirrorConfirmGate(name, path string, check func(map[string]any) error, preview, send server.ToolHandlerFunc) server.ToolHandlerFunc {
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
		if check != nil {
			if err := check(rest); err != nil {
				return boundedToolResultError(err.Error()), nil
			}
		}
		if preview != nil {
			res, err := preview(ctx, req)
			if res != nil {
				res.Content = append(res.Content, mcplib.NewTextContent(confirmNote))
			}
			return res, err
		}
		return argvPreview(name, path, rest, confirmNote), nil
	}
}

// argvPreview is a preview that ran nothing: the command and the arguments it
// would run with.
func argvPreview(name, path string, args map[string]any, note string) *mcplib.CallToolResult {
	text, err := bound.JSON(map[string]any{
		"preview":               true,
		"confirmation_required": true,
		"tool":                  name,
		"command":               path,
		"arguments":             args,
		"note":                  note,
	})
	if err != nil {
		return boundedToolResultError("encoding confirmation preview: " + err.Error())
	}
	return mcplib.NewToolResultText(text)
}

// unlessFlagsConfirmGate runs a command annotated mcp:read-only-unless-flags
// as mirrored while none of those flags is among the arguments (it is a read
// then). With one present, whatever its value, the call runs nothing until
// confirm is boolean true: the plan would mint a token and open a writable store
// (class 4), and the same call without the flag is that plan. An undeclared
// argument is refused by name first.
func unlessFlagsConfirmGate(name, path string, unless, allowed map[string]bool, next server.ToolHandlerFunc) server.ToolHandlerFunc {
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
		if err := validateMCPArgumentNames(rest, allowed); err != nil {
			return boundedToolResultError(err.Error()), nil
		}
		return argvPreview(name, path, rest, "Preview only: nothing ran. Call without "+flagNames(unless)+" for the plan, or again with the same arguments and confirm set to boolean true to send it."), nil
	}
}
