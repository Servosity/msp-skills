// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package store

import "netsapiens-pp-cli/internal/cliutil"

// Unit tests here do not hold what they store: nothing here runs credkit.Start,
// and every value held in this one process would be checked by every later test.
// Holding is proven on the built binaries by the leak tests in internal/credwire.
func init() {
	cliutil.Hold = func(string, ...string) {}
}
