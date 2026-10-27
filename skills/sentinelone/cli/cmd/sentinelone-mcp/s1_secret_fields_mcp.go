// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): the MCP server's tool-result guard for
// credentials, registered by one marked line in the generated main.go.

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"sentinelone-pp-cli/internal/cli"
	"sentinelone-pp-cli/internal/cliutil"
	"sentinelone-pp-cli/internal/store"
)

// confirmedExecuteTool is the one call that may return a credential: the
// execute tool with confirm set to boolean true, which is how a secret GET
// (agents.get-passphrase, accounts.uninstall-password.get, ...) is read over
// MCP at all (s1_secret_reads.go makes those need confirm).
const confirmedExecuteTool = "sentinelone_execute"

// liveAPITools never return local-store rows: the code-orchestration tools
// call the API, context describes it. Every other tool (sql, search,
// analytics and the other mirrored commands) can read the local store, so it
// is refused while a store an older build wrote cannot be scrubbed.
var liveAPITools = map[string]bool{
	"sentinelone_search": true, "sentinelone_get": true, "sentinelone_read": true,
	"sentinelone_execute": true, "context": true,
}

var scrubbedStores sync.Map // store path -> struct{}

// withholdToolSecrets replaces every credential value (cliutil.
// WithholdSecretFieldsText: secret-named fields at any depth, name/value
// pairs, JSON inside strings, previews cut mid-value, key: value text) in
// every tool result before the client sees it: sentinelone_read (a site's
// registrationToken, settings passwords), sql, search, tail, export and every
// mirrored command alike. Only sentinelone_execute with confirm: true returns
// the answer as the API sent it.
func withholdToolSecrets(next server.ToolHandlerFunc) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		// The first call scrubs the local store of anything an older build
		// synced, before any tool reads it.
		if err := scrubLocalStore(ctx); err != nil && !liveAPITools[req.Params.Name] {
			return mcplib.NewToolResultError(fmt.Sprintf("refusing %s: the local store may still hold credentials an older build synced and could not be scrubbed (%v). Run sentinelone-cli sync once to scrub it.", req.Params.Name, err)), nil
		}
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
			} else if out := cliutil.WithholdSecretFieldsText(string(b)); out != string(b) {
				var v any
				if json.Unmarshal([]byte(out), &v) != nil {
					res.StructuredContent = nil
				} else {
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

// scrubLocalStore opens the MCP store read-write once per path, which runs
// the store's migrations, including the one-time credential scrub
// (store.scrubStoredSecrets). A missing store has nothing to scrub. This is a
// local-only write, so it also runs under SENTINELONE_MCP_READ_ONLY=1.
func scrubLocalStore(ctx context.Context) error {
	path, err := cli.MCPStorePath()
	if err != nil {
		return err
	}
	if _, done := scrubbedStores.Load(path); done {
		return nil
	}
	if _, statErr := os.Stat(path); statErr != nil {
		if os.IsNotExist(statErr) {
			return nil
		}
		return statErr
	}
	db, err := store.OpenWithContext(ctx, path)
	if err != nil {
		return err
	}
	if err := db.Close(); err != nil {
		return err
	}
	scrubbedStores.Store(path, struct{}{})
	return nil
}
