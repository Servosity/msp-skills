// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cobratree

// jira-service-management handfix (jsm-mcp-mirror-write-confirm; the
// cisco-meraki and sibling-connector gate): a mirrored command that can change
// Jira Service Management data previews, and sends only
// with confirm set to boolean true, the contract every MCP write follows. Fail
// closed: only read-only tools, commands annotated mcp:local-write and the
// local commands in mirrorLocalOnlyCommands run without confirm.
//   - a command annotated mcp:read-only-unless-flags
//     runs as mirrored while none of those flags
//     is among the arguments, which is its own preview; with one, whatever its
//     value, the call runs without those flags unless confirm is boolean true;
//   - any other such command (request note) previews with the root --dry-run forced
//     on (the client then sends nothing; a caller dry-run value is replaced),
//     so the mirror's own argument checks still refuse a blocked argument by
//     name; confirm: true runs it as given.

import (
	"context"
	"sort"
	"strings"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"
)

// mirrorLocalOnlyCommands change no Jira Service Management data: they only
// read the API (export, sync, workflow archive, cmdb discover, cmdb reconcile)
// or never reach it; their writes land in the CLI's own files (cmdb discover
// --save: the config file). cmdb reconcile's --file is refused over MCP.
var mirrorLocalOnlyCommands = map[string]bool{
	"export":            true,
	"sync":              true,
	"workflow archive":  true,
	"learnings forget":  true,
	"learnings reject":  true,
	"learnings confirm": true,
	"playbook amend":    true,
	"teach":             true,
	"teach-lookup":      true,
	"teach-pattern":     true,
	"teach-playbook":    true,
	"cmdb discover":     true,
	"cmdb reconcile":    true,
}

const confirmNote = "Preview only: nothing was sent. Call again with the same arguments and confirm set to boolean true to send it."

// RequireConfirmForMirroredWrites wraps every mirrored tool that may change
// Jira Service Management data in a confirm gate. Call it after RegisterAll.
func RequireConfirmForMirroredWrites(s *server.MCPServer, root *cobra.Command) {
	for _, entry := range s.ListTools() {
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
		if found {
			if unless := readOnlyUnlessFlagNames(cmd); len(unless) > 0 {
				addConfirmProperty(&tool, "It is needed only with --"+strings.Join(sortedKeys(unless), ", --")+", which changes Jira Service Management data; without confirm such a call returns the preview and sends nothing. Any value other than boolean true, including the string \"true\", previews.")
				s.AddTool(tool, unlessFlagsConfirmGate(unless, entry.Handler))
				continue
			}
		}
		addConfirmProperty(&tool, "It can change data in your Jira Service Management site. Any other value, including the string \"true\", returns a preview and sends nothing.")
		s.AddTool(tool, dryRunConfirmGate(entry.Handler))
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

func sortedKeys(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// dryRunConfirmGate runs the command as given only with confirm boolean true;
// otherwise it runs it with the root --dry-run on, which sends nothing.
func dryRunConfirmGate(send server.ToolHandlerFunc) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		args := req.GetArguments()
		rest := make(map[string]any, len(args)+1)
		for k, v := range args {
			if k != "confirm" {
				rest[k] = v
			}
		}
		if confirmed, ok := args["confirm"].(bool); ok && confirmed {
			req.Params.Arguments = rest
			return send(ctx, req)
		}
		rest["dry-run"] = true
		req.Params.Arguments = rest
		res, err := send(ctx, req)
		if res != nil {
			res.Content = append(res.Content, mcplib.NewTextContent(confirmNote))
		}
		return res, err
	}
}

// unlessFlagsConfirmGate runs a command annotated mcp:read-only-unless-flags
// as mirrored while none of those flags is among the arguments. With one
// present, whatever its value, the call runs as the same arguments without
// those flags (the preview), and sends only with confirm boolean true.
func unlessFlagsConfirmGate(unless map[string]bool, next server.ToolHandlerFunc) server.ToolHandlerFunc {
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
