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
		return argumentTypeRefusal(entry.Tool, args)
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

// argumentTypeRefusal refuses an argument whose JSON type differs from the
// type its property declares, for the non-string types (object, array,
// number, integer, boolean), instead of letting a handler coerce it: a
// handler that reads params with a failed type assertion used to fall back
// to {} (dnsfilter_execute organizations.promote-to-msp with params given as
// the STRING "{\"dry_run\":true}" sent the live POST). A null value for a
// required property is refused too; a null optional property reads as
// absent, as every handler already treats it.
func argumentTypeRefusal(t mcplib.Tool, args map[string]any) string {
	required := map[string]bool{}
	for _, r := range t.InputSchema.Required {
		required[r] = true
	}
	names := make([]string, 0, len(args))
	for k := range args {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, name := range names {
		v := args[name]
		if v == nil {
			if required[name] {
				return fmt.Sprintf("MCP parameter %q for %s is required and cannot be null; nothing was run", name, t.Name)
			}
			continue
		}
		want := declaredPropertyType(t, name)
		if want == "" || jsonTypeMatches(v, want) {
			continue
		}
		return fmt.Sprintf("MCP parameter %q for %s must be a JSON %s, not %s; nothing was run", name, t.Name, want, jsonTypeName(v))
	}
	return ""
}

// declaredPropertyType returns the declared JSON type of a property when it is
// one the gate enforces (object, array, number, integer, boolean).
func declaredPropertyType(t mcplib.Tool, name string) string {
	var prop map[string]any
	if p, ok := t.InputSchema.Properties[name].(map[string]any); ok {
		prop = p
	} else if len(t.RawInputSchema) > 0 {
		var raw struct {
			Properties map[string]map[string]any `json:"properties"`
		}
		if json.Unmarshal(t.RawInputSchema, &raw) == nil {
			prop = raw.Properties[name]
		}
	}
	typ, _ := prop["type"].(string)
	switch typ {
	case "object", "array", "number", "integer", "boolean":
		return typ
	}
	return ""
}

func jsonTypeMatches(v any, want string) bool {
	switch want {
	case "object":
		_, ok := v.(map[string]any)
		return ok
	case "array":
		_, ok := v.([]any)
		return ok
	case "boolean":
		_, ok := v.(bool)
		return ok
	case "number":
		switch v.(type) {
		case float64, json.Number:
			return true
		}
	case "integer":
		switch n := v.(type) {
		case float64:
			return n == float64(int64(n))
		case json.Number:
			_, err := n.Int64()
			return err == nil
		}
	}
	return false
}

func jsonTypeName(v any) string {
	switch v.(type) {
	case map[string]any:
		return "an object"
	case []any:
		return "an array"
	case string:
		return "a string"
	case bool:
		return "a boolean"
	case float64, json.Number:
		return "a number"
	}
	return fmt.Sprintf("a %T", v)
}
