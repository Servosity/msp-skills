// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/dnsfilter/handfixes.json
// under the "mcp-tools-refuse-undeclared-arguments" entry.

package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// installArgumentGate refuses, for EVERY registered tool (typed tools, code
// orchestration, the recipe intent and every Cobra-tree mirror, including
// tools a later registrar adds), a call that carries a top-level argument the
// tool's declared input schema does not list, before the handler or the
// tenant gate runs. mcp-go does not validate arguments against the schema by
// default, so a handler that reads only the keys it knows silently ignored
// the rest: {"endpoint_id":"networks.destroy","params":{"id":7},"dry_run":true}
// sent DELETE /v1/networks/7 although the caller asked for a preview. Read
// tools are refused too: an ignored argument is a misunderstanding either way,
// and one rule leaves nothing to decide per tool.
func installArgumentGate(s *server.MCPServer) {
	s.Use(func(next server.ToolHandlerFunc) server.ToolHandlerFunc {
		return func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
			if msg := undeclaredArgumentRefusal(s, req); msg != "" {
				return mcplib.NewToolResultError(msg), nil
			}
			return next(ctx, req)
		}
	})
}

// undeclaredArgumentRefusal returns a refusal when req's arguments are not a
// JSON object or name a key outside the tool's declared properties.
func undeclaredArgumentRefusal(s *server.MCPServer, req mcplib.CallToolRequest) string {
	entry := s.GetTool(req.Params.Name)
	if entry == nil || req.Params.Arguments == nil {
		return ""
	}
	args, ok := req.Params.Arguments.(map[string]any)
	if !ok {
		return fmt.Sprintf("arguments for %s must be a JSON object of the tool schema's named parameters; nothing was run", req.Params.Name)
	}
	declared := declaredToolProperties(entry.Tool)
	var unknown []string
	for k := range args {
		if !declared[k] {
			unknown = append(unknown, k)
		}
	}
	if len(unknown) == 0 {
		return ""
	}
	sort.Strings(unknown)
	quoted := make([]string, len(unknown))
	for i, k := range unknown {
		quoted[i] = strconv.Quote(k)
	}
	noun := "parameter"
	if len(unknown) > 1 {
		noun = "parameters"
	}
	return fmt.Sprintf("unknown MCP %s %s for %s; use the tool schema's named parameters (this server has no dry-run or confirm switch outside them); nothing was run", noun, strings.Join(quoted, ", "), req.Params.Name)
}

// declaredToolProperties lists the top-level property names of a tool's input
// schema, from the structured schema or a raw JSON schema.
func declaredToolProperties(t mcplib.Tool) map[string]bool {
	out := map[string]bool{}
	for k := range t.InputSchema.Properties {
		out[k] = true
	}
	if len(t.RawInputSchema) > 0 {
		var raw struct {
			Properties map[string]json.RawMessage `json:"properties"`
		}
		if json.Unmarshal(t.RawInputSchema, &raw) == nil {
			for k := range raw.Properties {
				out[k] = true
			}
		}
	}
	return out
}
