// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/synology/handfixes.json
// under the "mcp-required-args-enforced" entry.

package mcp

import (
	"context"
	"fmt"
	"sort"
	"strings"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// installRequiredArgsGuard makes every tool on the server enforce its own
// input schema's "required" list before its handler runs, so nothing is sent
// to the NAS (and nothing local runs) for a call that omits one.
//
// mcp-go advertises the schema but does not enforce it, and the generated
// endpoint handler forwards whatever arguments it got: abb_backup and
// abb_cancel declare task_ids as required, yet a call with no task_ids sent
// the write (GET ...method=backup&trigger_type=1 with no task list), while
// the CLI refuses the same call. The guard is a server middleware, so it
// covers every tool (endpoint tools, reads and writes alike, the Cobra-tree
// mirror and the novel tools) from the one schema the client sees.
func installRequiredArgsGuard(s *server.MCPServer) {
	s.Use(func(next server.ToolHandlerFunc) server.ToolHandlerFunc {
		return func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
			entry := s.GetTool(req.Params.Name)
			if entry == nil {
				return next(ctx, req)
			}
			if missing := missingRequiredArgs(entry.Tool.InputSchema.Required, req.GetArguments()); len(missing) > 0 {
				return mcpToolError(fmt.Sprintf("%s requires %s (missing or empty); nothing was sent", req.Params.Name, quotedList(missing))), nil
			}
			return next(ctx, req)
		}
	})
}

// missingRequiredArgs lists the required arguments a call omits or passes
// empty: absent, JSON null, or a string with nothing but white space. A
// number (0 included), a boolean (false included) or a non-empty value
// counts as given.
func missingRequiredArgs(required []string, args map[string]any) []string {
	var missing []string
	for _, name := range required {
		v, ok := args[name]
		switch tv := v.(type) {
		case nil:
			ok = false
		case string:
			ok = ok && strings.TrimSpace(tv) != ""
		}
		if !ok {
			missing = append(missing, name)
		}
	}
	sort.Strings(missing)
	return missing
}

func quotedList(names []string) string {
	q := make([]string, len(names))
	for i, n := range names {
		q[i] = fmt.Sprintf("%q", n)
	}
	return strings.Join(q, ", ")
}
