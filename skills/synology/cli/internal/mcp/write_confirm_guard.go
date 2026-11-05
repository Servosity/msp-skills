// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): HAND-FIX(mcp-writes-need-confirm).

package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// confirmArg is the argument a state-changing tool needs, as boolean true,
// before it does anything.
const confirmArg = "confirm"

// needsConfirm reports a typed (non-mirror) tool that is not annotated
// read-only: an endpoint write such as abb_backup. A typed tool with no
// annotation needs confirm: absence fails closed.
func needsConfirm(t mcplib.Tool) bool {
	a := t.Annotations
	return a.ReadOnlyHint == nil || !*a.ReadOnlyHint
}

// typedToolNames is the set of tools registered so far; RegisterTools takes
// it just before the Cobra-tree mirror, whose commands shell out to the CLI.
// The mirror never carries a DSM write: every endpoint command is a typed
// tool (TestMirrorCarriesNoDSMWrite keeps it that way).
func typedToolNames(s *server.MCPServer) map[string]bool {
	out := map[string]bool{}
	for name := range s.ListTools() {
		out[name] = true
	}
	return out
}

// installWriteConfirmGuard makes every state-changing tool preview first:
// without "confirm": true (a JSON boolean; "true" the string does not count)
// the call returns what it would do and sends nothing. The tool schema
// advertises confirm, so an agent sees the contract before calling, and the
// argument is removed before the tool's own handler runs (it is never
// forwarded to the NAS or to the companion CLI). abb_backup, abb_cancel and
// session_logout start or cancel a backup or end the DSM session on the NAS;
// the generated endpoint handler sent them on the first call.
//
// The gate wraps each tool's handler (not a server middleware), so every
// path that reaches the handler meets it. An argument the schema does not
// declare, or a missing required one, is refused before any preview.
// Call it after every tool is registered, with the typed tool names.
func installWriteConfirmGuard(s *server.MCPServer, typed map[string]bool) {
	for name, st := range s.ListTools() {
		if !typed[name] || !needsConfirm(st.Tool) {
			continue
		}
		tool := st.Tool
		props := map[string]any{}
		for k, v := range tool.InputSchema.Properties {
			props[k] = v
		}
		props[confirmArg] = map[string]any{
			"type":        "boolean",
			"description": "Set to true to run this state-changing call. Without it the tool returns a preview and sends nothing.",
		}
		tool.InputSchema.Properties = props
		tool.Description += " Changes state on the NAS: call once without confirm to preview, then with confirm: true to run."
		s.AddTool(tool, confirmGated(name, tool, st.Handler))
	}
}

func confirmGated(name string, tool mcplib.Tool, next server.ToolHandlerFunc) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		args := req.GetArguments()
		rest := make(map[string]any, len(args))
		var unknown []string
		for k, v := range args {
			if k == confirmArg {
				continue
			}
			if _, declared := tool.InputSchema.Properties[k]; !declared {
				unknown = append(unknown, k)
			}
			rest[k] = v
		}
		if len(unknown) > 0 {
			sort.Strings(unknown)
			return mcpToolError(fmt.Sprintf("%s does not take %s; use the tool schema's named parameters (nothing was sent)", name, quotedList(unknown))), nil
		}
		if missing := missingRequiredArgs(tool.InputSchema.Required, rest); len(missing) > 0 {
			return mcpToolError(fmt.Sprintf("%s requires %s (missing or empty); nothing was sent", name, quotedList(missing))), nil
		}
		if ok, isBool := args[confirmArg].(bool); !isBool || !ok {
			return writePreview(name, rest), nil
		}
		req.Params.Arguments = rest
		return next(ctx, req)
	}
}

// writePreview is the answer to an unconfirmed state-changing call.
func writePreview(tool string, args map[string]any) *mcplib.CallToolResult {
	keys := make([]string, 0, len(args))
	for k := range args {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	ordered := make([]map[string]any, 0, len(keys))
	for _, k := range keys {
		ordered = append(ordered, map[string]any{"name": k, "value": args[k]})
	}
	b, err := json.Marshal(map[string]any{
		"dry_run":   true,
		"tool":      tool,
		"arguments": ordered,
		"would":     fmt.Sprintf("run %s with these arguments", tool),
		"note":      "nothing was sent; call again with \"confirm\": true (a JSON boolean) to run it",
	})
	if err != nil {
		return mcpResultError("preview failed; nothing was sent")
	}
	return mcpResultText(string(b))
}
