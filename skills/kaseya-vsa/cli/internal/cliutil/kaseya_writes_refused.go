// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cliutil

// Hand-written (NOT generated; handfixes.json read-only-tools-change-nothing):
// an MCP tool annotated read-only skips the confirm gate, so its CLI child may
// change nothing on this machine. A child refuses writes from its first
// instruction, a flag that fails to parse included, until the parsed command
// turns out to be one that may write. Under the refusal the store opens
// read-only and keeps its file modes, and the journal and the audit and teach
// logs are not written.

import "sync/atomic"

var childWritesAllowed, mcpServer atomic.Bool

// AllowChildWrites records whether the parsed command may write.
func AllowChildWrites(allowed bool) { childWritesAllowed.Store(allowed) }

// MarkMCPServer records that this process is the MCP server. It answers read
// tools and unconfirmed previews itself, and every write it makes goes to the
// vendor or through a CLI child, so it changes no local file of its own.
func MarkMCPServer() { mcpServer.Store(true) }

// WritesRefused reports whether this process is the MCP server, or an MCP
// child that has not resolved a command allowed to write.
func WritesRefused() bool {
	return mcpServer.Load() || IsMCPChildEnv() && !childWritesAllowed.Load()
}
