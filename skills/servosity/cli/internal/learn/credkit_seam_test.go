// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package learn_test

import "servosity-msp-pp-cli/internal/cliutil"

// In-process tests hold no config value, which one test would carry into the
// next (see cliutil.HoldConfigFile); the held pass holds its own.
func init() {
	cliutil.HoldConfigFile = func([]byte) {}
}
