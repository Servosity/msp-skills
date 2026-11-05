// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cobratree

// dropsuite handfix (mcp-mirror-write-confirm): every MCP tool that can change
// Dropsuite state previews, and sends only with confirm set to boolean true.
// Dropsuite's API is read-only for reseller credentials except the two portal
// sign-in link calls (GET /users/generate_redirect_link and
// /users/generate_support_sso_link): each mints a login link and invalidates
// the previous one. They are reached through the typed endpoint tools
// (portal_get-login-link, organizations_get-support-sso-link) and through
// evidence --with-sso-link and locate --sso-link. Fail closed: only read-only
// tools, commands annotated mcp:local-write, and the local commands in
// mirrorLocalOnlyCommands run without confirm; any other endpoint tool
// previews its request and sends nothing until confirm is boolean true.
//
// Every preview runs the mirrored command through the mirror's own argument
// checks (a blocked or unknown argument is refused by name), sending nothing:
//   - a command annotated mcp:read-only-unless-flags runs as mirrored while
//     no such flag is among the arguments; with one, its preview is the same
//     call without those flags (the plan), and confirm: true runs it as given;
//   - a command annotated ds:preview-flag previews with that read-only flag
//     added, and confirm: true runs the arguments as given (no dropsuite
//     command uses it today);
//   - a command annotated ds:sends-with-yes previews by itself until
//     --yes: its preview is the command as mirrored, and confirm: true runs the
//     same arguments with --yes added by this gate (--yes stays a blocked tool
//     argument);
//   - any other command previews with the root --dry-run (the client then
//     sends nothing), and confirm: true runs it as mirrored.
// A preview sets its own flag: a caller argument naming it (plan, dry-run) is
// dropped from the preview whatever its type or value, since on the child's
// command line it would follow the injected flag and turn it off.
// A tool whose command is not in the tree echoes its arguments instead.

import (
	"context"
	"sort"
	"strings"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"

	"dropsuite-pp-cli/internal/mcp/bound"
)

// SendsWithYesAnnotation marks a command that previews until --yes.
const SendsWithYesAnnotation = "ds:sends-with-yes"

// PreviewFlagAnnotation names a read-only boolean flag of the command that
// turns a run into its own preview.
const PreviewFlagAnnotation = "ds:preview-flag"

// mirrorLocalOnlyCommands change no Dropsuite state: they only read (export,
// sync, workflow archive) or never reach the API (learnings forget/reject);
// their writes land in the CLI's own files.
var mirrorLocalOnlyCommands = map[string]bool{
	"export":           true,
	"sync":             true,
	"workflow archive": true,
	"learnings forget": true,
	"learnings reject": true,
}

const confirmNote = "Preview only: nothing was sent. Call again with the same arguments and confirm set to boolean true to send it."

// RequireConfirmForMirroredWrites wraps every mirrored tool and every endpoint
// tool that may change Dropsuite state in a confirm gate. Call it after
// RegisterAll.
func RequireConfirmForMirroredWrites(s *server.MCPServer, root *cobra.Command, cliPath func() (string, error)) {
	cliPath = shareCLIPath(cliPath)
	for name, entry := range s.ListTools() {
		tool := entry.Tool
		if hint := tool.Annotations.ReadOnlyHint; hint != nil && *hint {
			continue
		}
		if tool.Meta == nil || tool.Meta.AdditionalFields["pp:tenant-gate"] != "child-cli" {
			// An in-process endpoint tool that is not marked read-only: its
			// preview is the request it would send.
			addConfirmProperty(&tool, "It can change state in Dropsuite (a portal sign-in link call invalidates the previous link). Any other value, including the string \"true\", returns a preview and sends nothing.")
			s.AddTool(tool, mirrorConfirmGate(name, name, nil, entry.Handler, nil))
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
		// owned: the arguments the preview sets itself. A caller value for one
		// (plan "false", dry-run 0) would follow the injected flag on the
		// child's command line and turn the preview into a send, so the
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
			if unless := readOnlyUnlessFlagNames(cmd); len(unless) > 0 {
				addConfirmProperty(&tool, "It is needed only with "+flagNames(unless)+", which can change state in Dropsuite (a portal sign-in link invalidates the previous one); without confirm such a call returns the plan the command prints without "+flagNames(unless)+" and sends nothing. Any value other than boolean true, including the string \"true\", previews.")
				s.AddTool(tool, unlessFlagsConfirmGate(name, path, unless, entry.Handler))
				continue
			}
			if pf := strings.TrimSpace(cmd.Annotations[PreviewFlagAnnotation]); pf != "" && previewFlagIsBool(cmd, pf) {
				preview, owned = run("--"+pf), []string{pf, "dry-run"}
			} else if annotationIsTrue(cmd, SendsWithYesAnnotation) {
				preview, send = entry.Handler, run("--"+confirmationBypassYesFlag)
			} else {
				preview, owned = run("--dry-run"), []string{"dry-run"}
			}
		}
		addConfirmProperty(&tool, "It can change state in Dropsuite. Any other value, including the string \"true\", returns a preview and sends nothing.")
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

func flagNames(set map[string]bool) string {
	names := make([]string, 0, len(set))
	for n := range set {
		names = append(names, "--"+n)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
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
		req.Params.Arguments = rest
		if confirmed, ok := args["confirm"].(bool); ok && confirmed {
			return send(ctx, req)
		}
		if preview != nil {
			planArgs := make(map[string]any, len(rest))
			for k, v := range rest {
				planArgs[k] = v
			}
			for _, k := range owned {
				delete(planArgs, k)
			}
			req.Params.Arguments = planArgs
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

// previewFlagIsBool reports a boolean flag the command itself declares; any
// other annotation value falls back to the --dry-run preview (fail closed).
func previewFlagIsBool(cmd *cobra.Command, name string) bool {
	f := cmd.LocalFlags().Lookup(name)
	return f != nil && f.Value.Type() == "bool"
}
