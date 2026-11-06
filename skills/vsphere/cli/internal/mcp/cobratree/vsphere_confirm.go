// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cobratree

// vsphere handfix (mcp-mirror-write-confirm; Codex r1 class
// mcp-write-without-confirm): every MCP tool that is not a proven read
// previews, and acts only with confirm set to boolean true, the contract
// vsphere_execute has. That covers a command that can change vCenter state
// (none today: every vsphere command only reads vCenter; this gate catches any
// command added later), every command that writes this CLI's own files
// (learnings, playbooks, teach notes, the local store, an export), and any
// tool nothing classifies. Fail closed: a tool runs without confirm only when
// it is marked read-only AND that mark is proven (a mirrored tool's command
// carries mcp:read-only, or is read-only-unless-flags with the MCP read-only
// switch on, so its write flags are dropped); a typed tool outside the mirror
// runs ungated only when it is marked read-only. A tool that claims read-only
// without proof loses the mark and is gated, so the read-only switch drops it.
//
// A preview never starts the CLI unless the command declares how to preview
// without writing anything:
//   - a command annotated mcp:read-only-unless-flags runs as mirrored while
//     no such flag is among the arguments; with one, its preview is the same
//     call without those flags (the plan), and confirm: true runs it as given;
//   - a command annotated vsphere:preview-flag (e.g. a --plan flag) previews with that
//     read-only flag added (the command reports what it would do, changing
//     nothing), and confirm: true runs the
//     arguments as given;
//   - a command annotated vsphere:sends-with-yes previews by itself until
//     --yes: its preview is the command as mirrored, and confirm: true runs the
//     same arguments with --yes added by this gate (--yes stays a blocked tool
//     argument);
//   - a command annotated vsphere:dry-run-preview (the root --dry-run makes it
//     change nothing, in vCenter or on disk) previews with --dry-run, and
//     confirm: true runs it as mirrored.
//   - a local writer listed in LocalWriterDryRunPreviews (its --dry-run is
//     proven to change no file and send nothing, by
//     TestLocalWriterPreviewsChangeNothing in vsphereblackbox) previews with
//     --dry-run, so ordinary flags reach the CLI, and confirm: true runs it as
//     mirrored.
//
// Every other tool, including any other local writer (mcp:local-write, or named
// in mirrorLocalWriters, whatever else it declares), an unclassified command, a
// tool whose command is not in the tree, and a typed tool, previews by echoing
// the call (the tool, the command and the argument names, never a value) and
// starts nothing. The echo first refuses, by name, any argument the tool does
// not declare: a host-file flag (--config, --receipt-file) is not in a schema,
// and answering it with a preview reads as accepted.
// A preview sets its own flag: a caller argument naming it (plan, dry-run) is
// dropped from the preview whatever its type or value, since on the child's
// command line it would follow the injected flag and turn it off.
// Every run preview passes the mirrored command's own argument checks (a
// blocked or unknown argument is refused by name).

import (
	"context"
	"sort"
	"strings"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"

	"vsphere-pp-cli/internal/cliutil"
	"vsphere-pp-cli/internal/mcp/bound"
)

// SendsWithYesAnnotation marks a command that previews until --yes.
const SendsWithYesAnnotation = "vsphere:sends-with-yes"

// PreviewFlagAnnotation names a read-only boolean flag of the command that
// turns a run into its own preview (a --plan flag).
const PreviewFlagAnnotation = "vsphere:preview-flag"

// DryRunPreviewAnnotation marks a command that changes nothing (in vCenter or
// on disk) when run with the root --dry-run, so that run is its preview.
const DryRunPreviewAnnotation = "vsphere:dry-run-preview"

// mirrorLocalWriters write the CLI's own files (the local store, an export,
// learnings); they never reach a vCenter write. They are gated like any
// writer and always preview by echoing the call, whatever they declare.
var mirrorLocalWriters = map[string]bool{
	"export":           true,
	"sync":             true,
	"workflow archive": true,
	"learnings forget": true,
	"learnings reject": true,
}

// LocalWriterDryRunPreviews are the local writers whose --dry-run is proven
// free of side effects. A command not listed here echoes its preview and starts
// nothing, so adding one needs a call in TestLocalWriterPreviewsChangeNothing.
var LocalWriterDryRunPreviews = map[string]bool{
	"export":            true,
	"learnings confirm": true,
	"learnings forget":  true,
	"learnings reject":  true,
	"playbook amend":    true,
	"sync":              true,
	"teach":             true,
	"teach-lookup":      true,
	"teach-pattern":     true,
	"teach-playbook":    true,
	"workflow archive":  true,
}

const confirmNote = "Preview only: nothing was sent or changed. Call again with the same arguments and confirm set to boolean true to run it."

// RequireConfirmForMirroredWrites wraps every registered tool that is not a
// proven read in a confirm gate. Call it after every tool is registered.
func RequireConfirmForMirroredWrites(s *server.MCPServer, root *cobra.Command, cliPath func() (string, error)) {
	cliPath = shareCLIPath(cliPath)
	readOnlyMode := cliutil.IsMCPReadOnlyEnv()
	for name, entry := range s.ListTools() {
		tool := entry.Tool
		marked := tool.Annotations.ReadOnlyHint != nil && *tool.Annotations.ReadOnlyHint
		mirrored := tool.Meta != nil && tool.Meta.AdditionalFields["pp:tenant-gate"] == "child-cli"
		if !mirrored {
			if marked {
				continue
			}
			addConfirmProperty(&tool, unclassifiedWhy)
			s.AddTool(tool, mirrorConfirmGate(name, name, nil, entry.Handler, nil, declaredArguments(tool)))
			continue
		}
		path, _ := tool.Meta.AdditionalFields[mirrorCLICommandMetaKey].(string)
		cmd, _, err := root.Find(strings.Fields(path))
		found := err == nil && cmd != root && cmd.CommandPath() == root.Name()+" "+path
		if marked && found && (isMCPReadOnly(cmd) || (readOnlyMode && len(readOnlyUnlessFlagNames(cmd)) > 0)) {
			continue
		}
		if marked {
			// The read-only mark is not proven: the tool is gated, the
			// read-only switch must drop it like any other writer, and hosts
			// must not read it as additive-only.
			notReadOnly := false
			tool.Annotations.ReadOnlyHint = &notReadOnly
			tool.Annotations.DestructiveHint = nil
		}
		localWrite := mirrorLocalWriters[path] || (found && isMCPLocalWrite(cmd))
		var preview, send server.ToolHandlerFunc
		// owned: the arguments the preview sets itself. A caller value for one
		// (plan "false", dry-run 0) would follow the injected flag on the
		// child's command line and turn the preview into a send, so the
		// preview drops them, whatever their type or value.
		var owned []string
		send = entry.Handler
		why := unclassifiedWhy
		if localWrite {
			why = localWriteWhy
		}
		if found && (!localWrite || LocalWriterDryRunPreviews[path]) {
			blocked := blockedStructuredArgsForCommand(cmd)
			positionals := positionalArgsForCommand(cmd, blocked)
			allowed := allowedStructuredArgsForCommand(cmd, blocked, positionals, commandTakesArgs(cmd))
			run := func(extra string) server.ToolHandlerFunc {
				return guardMirroredMCPCall(cmd, shellOutToCLI(cliPath, append(strings.Fields(path), extra),
					cliFlagBlockedArgs(blocked, positionals), allowed, positionals, false, positionalWriteSinkIndexes(cmd)))
			}
			if localWrite {
				preview, owned = run("--dry-run"), []string{"dry-run"}
				addConfirmProperty(&tool, why)
				s.AddTool(tool, mirrorConfirmGate(name, path, preview, send, owned, nil))
				continue
			}
			if unless := readOnlyUnlessFlagNames(cmd); len(unless) > 0 {
				addConfirmProperty(&tool, "It is needed only with "+flagNames(unless)+", which can change state in your vCenter; without confirm such a call returns the plan the command prints without "+flagNames(unless)+" and sends nothing. Any value other than boolean true, including the string \"true\", previews.")
				s.AddTool(tool, unlessFlagsConfirmGate(name, path, unless, entry.Handler))
				continue
			}
			switch pf := strings.TrimSpace(cmd.Annotations[PreviewFlagAnnotation]); {
			case pf != "" && previewFlagIsBool(cmd, pf):
				preview, owned = run("--"+pf), []string{pf, "dry-run"}
				why = vendorWriteWhy
			case annotationIsTrue(cmd, SendsWithYesAnnotation):
				preview, send = entry.Handler, run("--"+confirmationBypassYesFlag)
				why = vendorWriteWhy
			case annotationIsTrue(cmd, DryRunPreviewAnnotation):
				preview, owned = run("--dry-run"), []string{"dry-run"}
				why = vendorWriteWhy
			}
		}
		declared := declaredArguments(tool)
		addConfirmProperty(&tool, why)
		s.AddTool(tool, mirrorConfirmGate(name, path, preview, send, owned, declared))
	}
}

// The confirm parameter's reason, by what the tool may write.
const (
	vendorWriteWhy  = "It can change state in your vCenter. Any other value, including the string \"true\", returns a preview and sends nothing."
	localWriteWhy   = "It writes this CLI's own files (learnings, playbooks, the local store or an export); vCenter is only read. Any other value, including the string \"true\", returns a preview and changes nothing."
	unclassifiedWhy = "Nothing proves this tool only reads, so it is treated as a write. Any other value, including the string \"true\", returns a preview and changes nothing."
)

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

// declaredArguments is the tool's own schema, before the gate adds confirm.
func declaredArguments(tool mcplib.Tool) map[string]bool {
	declared := make(map[string]bool, len(tool.InputSchema.Properties))
	for k := range tool.InputSchema.Properties {
		declared[k] = true
	}
	return declared
}

// mirrorConfirmGate: declared applies to the echo preview; a run preview and
// send check their own arguments (shellOutToCLI, or the typed handler).
func mirrorConfirmGate(name, path string, preview, send server.ToolHandlerFunc, owned []string, declared map[string]bool) server.ToolHandlerFunc {
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
		if err := validateMCPArgumentNames(rest, declared); err != nil {
			return boundedToolResultError(err.Error()), nil
		}
		// The echo names the arguments and never repeats a value: a value the
		// server learns is a credential later in the session (a session id
		// minted by a concurrent call) must not already be in a result.
		names := make([]string, 0, len(rest))
		for k := range rest {
			names = append(names, k)
		}
		sort.Strings(names)
		text, err := bound.JSON(map[string]any{
			"preview":               true,
			"confirmation_required": true,
			"tool":                  name,
			"command":               path,
			"argument_names":        names,
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
