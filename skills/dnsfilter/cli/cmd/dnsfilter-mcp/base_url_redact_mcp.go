// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): the MCP server's error results never show a
// URL's credentials. Pinned by this connector's handfixes.json under the
// "base-url-credentials-held-and-redacted" entry (class "secret-echo").

package main

import (
	"context"
	"errors"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"dnsfilter-pp-cli/internal/cliutil"
)

// redactToolErrorURLs shows every URL in an error result (IsError) or a
// handler error with its credentials redacted (cliutil.RedactURLCredentials),
// the twin of the CLI's redactCommandErrors. main registers it right after
// credkit.MCPTools, so a held password yields the redacted text, not a withheld
// result.
func redactToolErrorURLs(next server.ToolHandlerFunc) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		res, err := next(ctx, req)
		if err != nil {
			if msg := cliutil.RedactURLCredentials(err.Error()); msg != err.Error() {
				err = errors.New(msg)
			}
		}
		if res != nil && res.IsError {
			for i, c := range res.Content {
				switch tc := c.(type) {
				case mcplib.TextContent:
					tc.Text = cliutil.RedactURLCredentials(tc.Text)
					res.Content[i] = tc
				case *mcplib.TextContent:
					if tc != nil {
						tc.Text = cliutil.RedactURLCredentials(tc.Text)
					}
				}
			}
		}
		return res, err
	}
}
