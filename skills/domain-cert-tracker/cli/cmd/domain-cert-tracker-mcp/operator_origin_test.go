// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package main

import (
	"testing"

	"domain-cert-tracker-pp-cli/internal/cli"
)

func TestMCPServerIsMarked(t *testing.T) {
	if !cli.MCPServerMarked() {
		t.Fatal("the MCP server binary does not mark itself, so a config-file base_url would widen the guard for tool calls")
	}
}
