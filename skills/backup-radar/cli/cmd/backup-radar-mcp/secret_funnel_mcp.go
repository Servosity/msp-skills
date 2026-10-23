// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): the MCP server's error backstop for the shared
// secret-echo funnel. Pinned by this connector's handfixes.json under the
// "secret-echo-decoded-funnel" entry (class "secret-echo").

package main

import (
	"context"
	"encoding/json"
	"errors"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"backup-radar-pp-cli/internal/cliutil"
)

// funnelToolErrors is the outermost tool-handler middleware: an error result
// (IsError) and a handler error pass the shared funnel (cliutil.FunnelHeld)
// before they reach the client. Success results are never funnelled.
func funnelToolErrors(next server.ToolHandlerFunc) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		res, err := next(ctx, req)
		if err != nil {
			if msg := cliutil.FunnelHeld(err.Error()); cliutil.FunnelWithheld(msg) {
				err = errors.New(msg)
			}
		}
		if res != nil && res.IsError {
			for i, c := range res.Content {
				switch tc := c.(type) {
				case mcplib.TextContent:
					tc.Text = cliutil.FunnelHeld(tc.Text)
					res.Content[i] = tc
				case *mcplib.TextContent:
					tc.Text = cliutil.FunnelHeld(tc.Text)
				}
			}
			if res.StructuredContent != nil {
				if b, mErr := json.Marshal(res.StructuredContent); mErr != nil || cliutil.FunnelWithheld(cliutil.FunnelHeld(string(b))) {
					res.StructuredContent = nil
				}
			}
		}
		return res, err
	}
}
