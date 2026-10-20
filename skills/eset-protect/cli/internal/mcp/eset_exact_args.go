// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"regexp"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// ExactArguments (handfixes.json: json-numbers-exact) re-decodes a tool
// call's wire arguments with json.Number. mcp-go fills Params.Arguments with
// a plain json.Unmarshal, so 9007199254740993 in an execute body or a CLI
// flag reached ESET as 9007199254740992. Every GetArguments caller now sees
// the exact number; fmt %v and json.Marshal both print it digit for digit.
func ExactArguments(next server.ToolHandlerFunc) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		if len(req.Params.RawArguments) > 0 {
			dec := json.NewDecoder(bytes.NewReader(req.Params.RawArguments))
			dec.UseNumber()
			var args map[string]any
			if dec.Decode(&args) == nil && args != nil {
				req.Params.Arguments = exactIntegersOnly(args)
			}
		}
		return next(ctx, req)
	}
}

// exactIntegersOnly keeps json.Number only for plain integer literals (the
// values float64 can round). Every other number (7.0, 1e1, 1.5) goes back to
// float64 as mcp-go decoded it, because integer flags and query params reject
// the literal "7.0" that json.Number would print.
func exactIntegersOnly(v any) any {
	switch t := v.(type) {
	case json.Number:
		if plainIntegerRE.MatchString(string(t)) {
			return t
		}
		if f, err := t.Float64(); err == nil {
			return f
		}
		return t
	case map[string]any:
		for k, x := range t {
			t[k] = exactIntegersOnly(x)
		}
		return t
	case []any:
		for i, x := range t {
			t[i] = exactIntegersOnly(x)
		}
		return t
	}
	return v
}

var plainIntegerRE = regexp.MustCompile(`^-?[0-9]+$`)

// argPositive reads a positive numeric argument (a limit) decoded either way.
func argPositive(args map[string]any, key string) (float64, bool) {
	v, ok := args[key].(float64)
	if n, isNumber := args[key].(json.Number); isNumber {
		f, err := n.Float64()
		v, ok = f, err == nil
	}
	return v, ok && v > 0
}
