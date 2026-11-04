// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cobratree

// mimecast hand-fix (handfixes.json mimecast-mcp-mirror-write-confirm): a
// mirrored command that can write (Mimecast state: held release-safe, held
// purge-sender, import; or the CLI's own files: jobs prune, learnings, teach,
// export, sync; and any command added later) previews, and runs only with
// confirm set to boolean true, the contract mimecast_execute has. Fail
// closed: only read-only tools run without confirm, and a read-only tool's
// write flags (mcp:read-only-unless-flags, held triage --new) need it too.
//
// A preview runs nothing that writes. A command that writes only local files
// (mcp:local-write, mirrorLocalOnlyCommands) echoes its arguments: the root
// --dry-run stops API sends, not file writes. Any other preview runs the
// mirrored command through the mirror's own argument checks (a blocked or
// unknown argument is refused by name), sending nothing:
//   - a command annotated mimecast:sends-with-yes previews by itself until
//     --yes (held purge-sender plans): its preview is the command as
//     mirrored, and confirm: true runs the same arguments with --yes added by
//     this gate (--yes stays a blocked tool argument);
//   - a command annotated mimecast:preview-flag previews with that flag
//     (held release-safe --plan classifies the IDs and releases nothing);
//   - any other command previews with the root --dry-run (the client then
//     sends nothing), and confirm: true runs it as mirrored.
// The preview flag is also a tool argument, so a preview drops the caller's
// value for it: "dry-run": false must not turn a preview into a send.
// A tool whose command is not in the tree echoes its arguments too, with every
// credential-named argument redacted.

import (
	"context"
	"sort"
	"strings"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"

	"mimecast-pp-cli/internal/mcp/bound"
)

// SendsWithYesAnnotation marks a command that previews until --yes.
const SendsWithYesAnnotation = "mimecast:sends-with-yes"

// PreviewFlagAnnotation names a command's own read-only preview flag.
const PreviewFlagAnnotation = "mimecast:preview-flag"

// mirrorLocalOnlyCommands change no Mimecast state: they send only GETs (export,
// sync, workflow archive) or never reach the API (learnings forget/reject,
// jobs prune); their writes land in the CLI's own files, which --dry-run does
// not stop, so they preview by echoing their arguments.
var mirrorLocalOnlyCommands = map[string]bool{
	"export":           true,
	"sync":             true,
	"workflow archive": true,
	"learnings forget": true,
	"learnings reject": true,
	"jobs prune":       true,
}

const confirmNote = "Preview only: nothing was sent or written. Call again with the same arguments and confirm set to boolean true to run it."

const confirmDescription = "Set to boolean true to run this command. It can change state in your Mimecast account (release or reject mail, create records) or in this CLI's local files (jobs ledger, learnings, snapshots). Any other value, including the string \"true\", returns a preview and runs nothing."

// RequireConfirmForMirroredWrites wraps every mirrored tool that may change
// Mimecast state in a confirm gate. Call it after RegisterAll.
func RequireConfirmForMirroredWrites(s *server.MCPServer, root *cobra.Command, cliPath func() (string, error)) {
	cliPath = shareCLIPath(cliPath)
	for name, entry := range s.ListTools() {
		tool := entry.Tool
		if tool.Meta == nil || tool.Meta.AdditionalFields["pp:tenant-gate"] != "child-cli" {
			continue
		}
		path, _ := tool.Meta.AdditionalFields[mirrorCLICommandMetaKey].(string)
		cmd, _, err := root.Find(strings.Fields(path))
		found := err == nil && cmd != root && cmd.CommandPath() == root.Name()+" "+path
		var writeFlags map[string]bool
		if hint := tool.Annotations.ReadOnlyHint; hint != nil && *hint {
			if !found {
				continue
			}
			if writeFlags = readOnlyUnlessFlagNames(cmd); len(writeFlags) == 0 {
				continue
			}
		}
		var preview, send server.ToolHandlerFunc
		send = entry.Handler
		local := mirrorLocalOnlyCommands[path] || writeFlags != nil || (found && isMCPLocalWrite(cmd))
		if found {
			blocked := blockedStructuredArgsForCommand(cmd)
			positionals := positionalArgsForCommand(cmd, blocked)
			allowed := allowedStructuredArgsForCommand(cmd, blocked, positionals, commandTakesArgs(cmd))
			run := func(extra string) server.ToolHandlerFunc {
				return guardMirroredMCPCall(cmd, shellOutToCLI(cliPath, append(strings.Fields(path), extra),
					cliFlagBlockedArgs(blocked, positionals), allowed, positionals, false, positionalWriteSinkIndexes(cmd)))
			}
			previewWith := func(flag string) server.ToolHandlerFunc {
				h := run("--" + flag)
				return func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
					args := req.GetArguments()
					rest := make(map[string]any, len(args))
					for k, v := range args {
						if k != flag {
							rest[k] = v
						}
					}
					req.Params.Arguments = rest
					return h(ctx, req)
				}
			}
			switch {
			case local:
				// --dry-run does not stop a local file write, but Cobra answers
				// --help after parsing and before any hook or RunE: the preview
				// runs the mirror's argument checks and writes nothing.
				preview = previewWith("help")
			case annotationIsTrue(cmd, SendsWithYesAnnotation):
				preview, send = entry.Handler, run("--"+confirmationBypassYesFlag)
			case cmd.Annotations[PreviewFlagAnnotation] != "":
				preview = previewWith(cmd.Annotations[PreviewFlagAnnotation])
			default:
				preview = previewWith("dry-run")
			}
		}
		if tool.InputSchema.Properties == nil {
			tool.InputSchema.Properties = map[string]any{}
		}
		tool.InputSchema.Properties["confirm"] = map[string]any{
			"type":        "boolean",
			"description": confirmDescription,
		}
		gate := mirrorConfirmGate(name, path, preview, send)
		if writeFlags != nil {
			tool.Description = strings.TrimSpace(tool.Description + " Its write flags (" + strings.Join(sortedNames(writeFlags), ", ") + ") return a preview and write nothing until confirm is boolean true.")
			gate = confirmWhenFlagged(writeFlags, gate, send)
		} else {
			tool.Description = strings.TrimSpace(tool.Description + " Returns a preview and runs nothing until confirm is boolean true.")
		}
		s.AddTool(tool, gate)
	}
}

// confirmWhenFlagged sends a read-only tool's call through the confirm gate
// when it sets any write flag to anything but boolean false; any other call
// runs as before (confirm, if passed, is dropped).
func confirmWhenFlagged(flags map[string]bool, gate, send server.ToolHandlerFunc) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		args := req.GetArguments()
		for name := range flags {
			if v, ok := args[name]; ok && v != false {
				return gate(ctx, req)
			}
		}
		rest := make(map[string]any, len(args))
		for k, v := range args {
			if k != "confirm" {
				rest[k] = v
			}
		}
		req.Params.Arguments = rest
		return send(ctx, req)
	}
}

func sortedNames(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for name := range set {
		out = append(out, "--"+name)
	}
	sort.Strings(out)
	return out
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
