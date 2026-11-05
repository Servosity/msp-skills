// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): this package's tests run in a throwaway home and
// never read the operator's real config.

package main

import (
	"os"
	"testing"

	"zammad-pp-cli/internal/cliutil/testenv"
)

func TestMain(m *testing.M) {
	os.Exit(testenv.RunSandboxed(m))
}
