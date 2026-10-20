// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package mcp

import (
	"bytes"
	"context"
	"encoding/json"

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
				req.Params.Arguments = args
			}
		}
		return next(ctx, req)
	}
}

// argPositive reads a positive numeric argument (a limit) decoded either way.
func argPositive(args map[string]any, key string) (float64, bool) {
	v, ok := args[key].(float64)
	if n, isNumber := args[key].(json.Number); isNumber {
		f, err := n.Float64()
		v, ok = f, err == nil
	}
	return v, ok && v > 0
}
