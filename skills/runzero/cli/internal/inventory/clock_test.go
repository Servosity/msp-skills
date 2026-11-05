// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package inventory

import (
	"os"
	"testing"
	"time"
)

// TestMain pins the package clock (runzero handfix inventory-fixed-clock): the
// fixtures and the code under test read one fixed "now", so a window such as
// TestCertsExpiring's days_left never depends on today's date or on how long
// the test took (it failed as `days_left = -6, want -5` when a second passed
// between the fixture and the query).
func TestMain(m *testing.M) {
	fixed := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	clock = func() time.Time { return fixed }
	os.Exit(m.Run())
}
