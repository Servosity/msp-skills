// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"eset-protect-pp-cli/internal/cliutil"
)

// HoldCredentialArguments is mcp-credential-arguments-held: a tool call's
// arguments are held as the call wrote them, with the kit's HoldBody, before a
// handler can preview or echo them. Kept beside it: a JSON document carried inside
// a string argument (--task-action-params arrives that way), which HoldBody does
// not open (.review/kit-findings.md, 2026-11-04).
func HoldCredentialArguments(next server.ToolHandlerFunc) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		raw := []byte(req.Params.RawArguments)
		if len(raw) == 0 {
			raw, _ = json.Marshal(req.GetArguments())
		}
		cliutil.HoldBody(raw)
		dec := json.NewDecoder(bytes.NewReader(raw))
		for {
			tok, err := dec.Token()
			if err != nil {
				break
			}
			// Only a string that is a JSON object or array: never prose.
			if s, ok := tok.(string); ok {
				if t := strings.TrimSpace(s); (strings.HasPrefix(t, "{") || strings.HasPrefix(t, "[")) && json.Valid([]byte(t)) {
					cliutil.HoldBody([]byte(t))
				}
			}
		}
		return next(ctx, req)
	}
}
