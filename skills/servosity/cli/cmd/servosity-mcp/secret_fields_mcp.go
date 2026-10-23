// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/servosity/handfixes.json under
// the "secret-family-never-persisted" entry. See docs/reprint-survival.md.

package main

import (
	"context"
	"encoding/json"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"servosity-msp-pp-cli/internal/cliutil"
)

// confirmedExecuteTool is the one call that may return a credential: the raw
// execute tool with confirm set to boolean true, the human-in-the-loop path
// governance.md names for secret reads.
const confirmedExecuteTool = "servosity-msp_execute"

// withholdToolSecrets replaces every secret-named field value in a tool
// result (cliutil.WithholdSecretValues) before the client sees it, for every
// tool: servosity-msp_read, sql, search, tail, analytics and the mirrored
// commands alike. Only servosity-msp_execute with confirm: true returns the
// value as the API sent it.
func withholdToolSecrets(next server.ToolHandlerFunc) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		res, err := next(ctx, req)
		if res == nil || confirmedSecretRead(req) {
			return res, err
		}
		for i, c := range res.Content {
			switch tc := c.(type) {
			case mcplib.TextContent:
				tc.Text = cliutil.WithholdSecretFieldsText(tc.Text)
				res.Content[i] = tc
			case *mcplib.TextContent:
				tc.Text = cliutil.WithholdSecretFieldsText(tc.Text)
			}
		}
		if res.StructuredContent != nil {
			if b, mErr := json.Marshal(res.StructuredContent); mErr != nil {
				res.StructuredContent = nil
			} else {
				var v any
				if json.Unmarshal(b, &v) != nil {
					res.StructuredContent = nil
				} else if cliutil.WithholdSecretValues(v) {
					res.StructuredContent = v
				}
			}
		}
		return res, err
	}
}

func confirmedSecretRead(req mcplib.CallToolRequest) bool {
	if req.Params.Name != confirmedExecuteTool {
		return false
	}
	confirm, ok := req.GetArguments()["confirm"].(bool)
	return ok && confirm
}
