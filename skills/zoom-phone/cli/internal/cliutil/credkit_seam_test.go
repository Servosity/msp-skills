// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cliutil

import "zoom-phone-pp-cli/internal/credkit"

// No test here runs credkit.Start, and the kit's held set is process-wide, so a value
// held by one test would withhold another's file writes. internal/zoomblackbox proves
// the hold on the built binaries.
func init() {
	Hold = func(credkit.Source, ...string) {}
}

// recordHolds swaps the hold seam for a recorder for one test.
func recordHolds(fn func()) []string {
	var got []string
	prior := Hold
	Hold = func(_ credkit.Source, values ...string) { got = append(got, values...) }
	defer func() { Hold = prior }()
	fn()
	return got
}
