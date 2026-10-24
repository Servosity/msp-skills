// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package mcp

import (
	"os"
	"testing"

	"cipp-pp-cli/internal/cliutil/testenv"
)

// TestMain sandboxes HOME and every CIPP_* path variable: executor tests build
// a client from config, which must never read or write the real user dirs.
func TestMain(m *testing.M) {
	os.Exit(testenv.RunSandboxed(m))
}
