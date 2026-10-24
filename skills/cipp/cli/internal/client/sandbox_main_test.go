// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"os"
	"testing"

	"cipp-pp-cli/internal/cliutil/testenv"
)

// TestMain sandboxes HOME and every CIPP_* path variable: the token tests mint
// and save, and the credentials file resolves through DataDir(), not beside
// cfg.Path, so an unsandboxed run writes into the real user data directory.
func TestMain(m *testing.M) {
	os.Exit(testenv.RunSandboxed(m))
}
