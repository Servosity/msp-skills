// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package main

import (
	"context"
	"net/http"
	"os"

	"acme-pp-cli/internal/cli"
	"acme-pp-cli/internal/credkit"

	"github.com/mark3labs/mcp-go/server"
)

func main() {
	credkit.Start(cli.Declarations)
	s := server.NewMCPServer("acme", "1", server.WithToolHandlerMiddleware(credkit.MCPTools))
	if len(os.Args) > 1 {
		srv := &http.Server{Addr: os.Args[1], Handler: credkit.MCPHTTP(server.NewStreamableHTTPServer(s))}
		_ = srv
		credkit.Exit(0)
	}
	_ = server.NewStdioServer(s).Listen(context.Background(), credkit.MCPIn(os.Stdin), credkit.MCPOut())
	credkit.Exit(0)
}
