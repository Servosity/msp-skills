// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package platform

import (
	"os"
	"testing"

	"rocketcyber-pp-cli/internal/cliutil"
	"rocketcyber-pp-cli/internal/credkit"
)

// TestMain keeps a credential one test resolves from being held for the rest of
// the process, where the kit would withhold every later test's file carrying it
// (handfixes.json credkit-wiring).
func TestMain(m *testing.M) {
	cliutil.Hold = func(credkit.Source, ...string) {}
	os.Exit(m.Run())
}
