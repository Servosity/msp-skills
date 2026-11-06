// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package rewstblackbox

import (
	"flag"
	"testing"
	"time"
)

// budgetStart is when this test binary started.
var budgetStart = time.Now()

// budgetReserve is what budget leaves before go test's -timeout: one running
// test has to finish inside it. The suite's longest test takes about a minute
// natively.
const budgetReserve = 3 * time.Minute

// budgetSpent reports whether a test starting now could outlive the -timeout.
// A limit of budgetReserve or less (including 0, no timeout) never skips.
func budgetSpent(limit, used time.Duration) bool {
	return limit > budgetReserve && used > limit-budgetReserve
}

// budget skips a test that would start too close to go test's -timeout. Every
// CLI run costs about 0.55 s of CPU with a credential held and a store open
// another 2 s (kit findings 13 and 14), and the suite runs serially: a slow
// machine (the amd64 emulation of verify_all's Linux row) would otherwise be
// killed at 10 minutes with a goroutine dump and no verdict for any test.
// Skipped tests are named in -v output. No test body changes, and a native run
// finishes inside the budget, so all of them run there.
func budget(t *testing.T) {
	t.Helper()
	f := flag.Lookup("test.timeout")
	if f == nil {
		return
	}
	limit, ok := f.Value.(flag.Getter).Get().(time.Duration)
	if !ok {
		return
	}
	if used := time.Since(budgetStart); budgetSpent(limit, used) {
		t.Skipf("time budget spent: %s of the %s -timeout used", used.Round(time.Second), limit)
	}
}

func TestBudgetSpent(t *testing.T) {
	for _, c := range []struct {
		limit, used time.Duration
		want        bool
	}{
		{10 * time.Minute, time.Minute, false},
		{10 * time.Minute, 7 * time.Minute, false},
		{10 * time.Minute, 7*time.Minute + time.Second, true},
		{0, time.Hour, false},
		{budgetReserve, time.Hour, false},
	} {
		if got := budgetSpent(c.limit, c.used); got != c.want {
			t.Errorf("budgetSpent(%s, %s) = %v, want %v", c.limit, c.used, got, c.want)
		}
	}
}
