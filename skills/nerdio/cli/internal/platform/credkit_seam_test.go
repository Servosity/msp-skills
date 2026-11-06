// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package platform

import (
	"os"
	"testing"

	"nerdio-pp-cli/internal/cliutil"
	"nerdio-pp-cli/internal/credkit"
)

// TestMain keeps a credential one test resolves from being held for the rest of
// the process, where the kit would withhold every later test's file carrying it.
func TestMain(m *testing.M) {
	cliutil.Hold = func(credkit.Source, ...string) {}
	os.Exit(m.Run())
}
