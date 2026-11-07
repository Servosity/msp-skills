// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package platform

import (
	"os"
	"testing"

	"abnormal-pp-cli/internal/cliutil"
)

// TestMain: no test here runs credkit.Start, so a value one test holds would
// withhold another test's file writes for the rest of the process.
func TestMain(m *testing.M) {
	cliutil.NoHolds()
	os.Exit(m.Run())
}
