// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// HAND-AUTHORED.

package mcp

import (
	"os"
	"testing"

	"synology-pp-cli/internal/cli"
)

// actAsCLIEnv makes this test binary run as synology-cli, so a test can point
// SYNOLOGY_CLI_PATH at os.Args[0] and drive the MCP command-mirror tools
// (which shell out to the companion CLI) end to end without a separate build.
const actAsCLIEnv = "SYNOLOGY_MCP_TEST_ACT_AS_CLI"

func TestMain(m *testing.M) {
	if os.Getenv(actAsCLIEnv) == "1" {
		if err := cli.Execute(); err != nil {
			os.Exit(cli.ExitCode(err))
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}
