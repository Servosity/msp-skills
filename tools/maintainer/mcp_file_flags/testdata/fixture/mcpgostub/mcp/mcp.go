// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Package mcp is a minimal stand-in for mark3labs/mcp-go's request type, so
// the analyzer fixture builds offline. The analyzer keys MCP tool handlers on
// the type name github.com/mark3labs/mcp-go/mcp.CallToolRequest.
package mcp

// CallToolRequest is the tool-call request an MCP caller sends.
type CallToolRequest struct {
	Arguments map[string]any
}

// GetString returns the string argument key, or def.
func (r CallToolRequest) GetString(key, def string) string {
	if s, ok := r.Arguments[key].(string); ok {
		return s
	}
	return def
}
