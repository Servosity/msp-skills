// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Package server stands in for mcp-go's server package: the names the checker knows.
package server

import (
	"context"
	"io"
	"net/http"
)

type MCPServer struct{}
type ServerOption func(*MCPServer)
type ToolHandlerFunc func(ctx context.Context, req any) (any, error)
type ToolHandlerMiddleware func(ToolHandlerFunc) ToolHandlerFunc

func NewMCPServer(name, version string, opts ...ServerOption) *MCPServer { return &MCPServer{} }
func WithToolHandlerMiddleware(m ToolHandlerMiddleware) ServerOption     { return nil }
func WithToolCapabilities(b bool) ServerOption                           { return nil }
func ServeStdio(s *MCPServer) error                                      { return nil }

type StdioServer struct{}

func NewStdioServer(s *MCPServer) *StdioServer                                       { return &StdioServer{} }
func (s *StdioServer) Listen(ctx context.Context, in io.Reader, out io.Writer) error { return nil }

type StreamableHTTPServer struct{}

func NewStreamableHTTPServer(s *MCPServer) *StreamableHTTPServer                 { return &StreamableHTTPServer{} }
func (s *StreamableHTTPServer) Start(addr string) error                          { return nil }
func (s *StreamableHTTPServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {}
