// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): no username or password from SUPEROPS_BASE_URL
// reaches an MCP client (superops-base-url-never-printed), and the stdio
// transport goes through the credential kit (credkit-wiring).

package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/signal"
	"syscall"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"superops-pp-cli/internal/cliutil"
	"superops-pp-cli/internal/config"
	"superops-pp-cli/internal/credkit"
)

// redactToolURLUserinfo removes any URL's userinfo from every tool result and
// handler error, success results included. Registered inside credkit.MCPTools,
// so the kit still sees what this leaves.
func redactToolURLUserinfo(next server.ToolHandlerFunc) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		res, err := next(ctx, req)
		if err != nil {
			if msg := cliutil.RedactURLUserinfo(err.Error()); msg != err.Error() {
				err = errors.New(msg)
			}
		}
		if res == nil {
			return res, err
		}
		for i, c := range res.Content {
			switch tc := c.(type) {
			case mcplib.TextContent:
				tc.Text = cliutil.RedactURLUserinfo(tc.Text)
				res.Content[i] = tc
			case *mcplib.TextContent:
				tc.Text = cliutil.RedactURLUserinfo(tc.Text)
			}
		}
		if res.StructuredContent != nil {
			if b, mErr := json.Marshal(res.StructuredContent); mErr != nil || cliutil.RedactURLUserinfo(string(b)) != string(b) {
				res.StructuredContent = nil
			}
		}
		return res, err
	}
}

// holdConfiguredCredentials holds every credential the config this server
// was started with names (config.Load holds them on every return), so a
// mirrored tool's error from the companion CLI meets them even before any
// in-process tool loads a config. A config that does not load is held as far
// as it was read; the error is the tool's to report.
func holdConfiguredCredentials() {
	_, _ = config.Load("")
}

// serveHeldStdio is server.ServeStdio with stdin and stdout through the kit.
func serveHeldStdio(s *server.MCPServer) error {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()
	return server.NewStdioServer(s).Listen(ctx, credkit.MCPIn(os.Stdin), credkit.MCPOut())
}
