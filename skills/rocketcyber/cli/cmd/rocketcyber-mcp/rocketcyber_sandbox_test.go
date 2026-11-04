// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package main

// rocketcyber handfix (mcp-tests-sandboxed): the MCP server and the CLI it
// runs write the config, state and cache directories, so these tests wrote
// into the operator's real home. Every test here runs under a sandboxed HOME,
// as internal/cli's do.

import (
	"os"
	"testing"

	"rocketcyber-pp-cli/internal/cliutil/testenv"
)

func TestMain(m *testing.M) {
	os.Exit(testenv.RunSandboxed(m))
}
