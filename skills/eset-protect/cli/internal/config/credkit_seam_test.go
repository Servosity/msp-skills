// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package config

import (
	"os"
	"testing"

	"eset-protect-pp-cli/internal/cliutil"
	"eset-protect-pp-cli/internal/credkit"
)

// TestMain runs this package's unit tests without holding: a config one test
// loads would withhold a later test's write of the same text. No test here runs
// credkit.Start; the leak tests in internal/credwire prove holding on the binaries.
func TestMain(m *testing.M) {
	cliutil.Hold = func(credkit.Source, ...string) {}
	cliutil.HoldBody = func([]byte) {}
	cliutil.HoldURL = func(credkit.Source, string) {}
	os.Exit(m.Run())
}
