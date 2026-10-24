// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated), cove only: the MCP half of the
// short-credential backstop. The shared backstop (secret_funnel_mcp.go,
// byte-identical across connectors and left untouched) looks only for held
// credentials of at least 8 characters; coveShortToolErrors, registered in
// main.go right after funnelToolErrors (so inside it), applies
// cliutil.FunnelHeldAll, which also withholds a shorter held credential, over
// the same surfaces: a handler error, and an error result's content and
// structured content. Hand-fix code: cove-short-credential-funnel.

package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"cove-pp-cli/internal/cliutil"
	"cove-pp-cli/internal/coverpc"
)

// The server process holds the Cove API token from the start, so the
// backstops know it before any client is built.
func init() {
	cliutil.FunnelHold(os.Getenv("COVE_PASSWORD"))
}

// coveShortToolErrors funnels an error result (IsError) and a handler error
// through cliutil.FunnelHeldAll. Before judging, it holds the API token and
// the visa the CLI children cached (they log in and rotate it), so both
// backstops withhold them. Success results are never funnelled here.
func coveShortToolErrors(next server.ToolHandlerFunc) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		res, err := next(ctx, req)
		cliutil.FunnelHold(os.Getenv("COVE_PASSWORD"), coverpc.CachedVisa())
		if err != nil {
			if msg := cliutil.FunnelHeldAll(err.Error()); cliutil.FunnelWithheld(msg) {
				err = errors.New(msg)
			}
		}
		if res != nil && res.IsError {
			for i, c := range res.Content {
				switch tc := c.(type) {
				case mcplib.TextContent:
					tc.Text = cliutil.FunnelHeldAll(tc.Text)
					res.Content[i] = tc
				case *mcplib.TextContent:
					tc.Text = cliutil.FunnelHeldAll(tc.Text)
				default:
					// Any other content is judged as its JSON form and
					// replaced whole when it repeats a held credential.
					if b, mErr := json.Marshal(c); mErr != nil || cliutil.FunnelWithheld(cliutil.FunnelHeldAll(string(b))) {
						res.Content[i] = mcplib.NewTextContent(cliutil.FunnelWithheldText)
					}
				}
			}
			if res.StructuredContent != nil {
				if b, mErr := json.Marshal(res.StructuredContent); mErr != nil || cliutil.FunnelWithheld(cliutil.FunnelHeldAll(string(b))) {
					res.StructuredContent = nil
				}
			}
		}
		return res, err
	}
}
