// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// cli-printing-press: novel-scaffold-test
// Novel command scaffold tests. Keep the wiring smoke test and add behavior cases as needed.

package cli

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"cork-pp-cli/internal/cliutil/testenv"
)

// TestNovelScoreRegressionsHelpWires smoke-tests that the score regressions command
// resolves at runtime and renders useful --help output. Catches wiring
// regressions (missing AddCommand, panicking RunE on --help, etc.) before
// review. Keep this smoke test when adding behavior-specific cases.
func TestNovelScoreRegressionsHelpWires(t *testing.T) {
	testenv.Isolate(t)
	cmd := RootCmd()
	cmd.SetArgs([]string{"score", "regressions", "--help"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("score regressions --help error = %v (novel command not wired correctly?)", err)
	}
	help := out.String()
	for _, want := range []string{"Usage:", "regressions"} {
		if !strings.Contains(help, want) {
			t.Fatalf("score regressions --help missing %q in output:\n%s", want, help)
		}
	}
}

// A client at 800 yesterday and 600 today, read with --since 24h: the move happened
// inside the window, so the score at window open (the last point before it) is the
// baseline. Comparing only in-window points reported "none dropped".
func TestCorkScoreMoveUsesScoreAtWindowOpen(t *testing.T) {
	now := time.Now()
	ts := func(d time.Duration) string { return now.Add(-d).UTC().Format(time.RFC3339) }
	hist := []corkScorePoint{{Score: 600, CreatedAt: ts(time.Hour)}, {Score: 800, CreatedAt: ts(25 * time.Hour)}}
	m := corkScoreMove(hist, now.Add(-24*time.Hour))
	if !m.ok || m.then.Score != 800 || m.now.Score != 600 || !m.hasBase {
		t.Fatalf("800 -> 600 inside the window not measured: %+v", m)
	}
	// One point only, no baseline: nothing to compare, so the client is unverified.
	if m := corkScoreMove(hist[:1], now.Add(-24*time.Hour)); m.ok {
		t.Fatalf("a single point with no baseline must not count as compared: %+v", m)
	}
	// No point inside the window but a baseline exists: the score held, delta 0.
	if m := corkScoreMove(hist[1:], now.Add(-2*time.Hour)); !m.ok || m.then.Score != m.now.Score {
		t.Fatalf("a held score must compare as unchanged: %+v", m)
	}
}
