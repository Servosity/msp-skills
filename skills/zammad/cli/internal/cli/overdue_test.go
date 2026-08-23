// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// cli-printing-press: novel-scaffold-test
// Novel command scaffold tests. Keep the wiring smoke test and add behavior cases as needed.

package cli

import (
	"bytes"
	"strings"
	"testing"
)

// TestNovelOverdueHelpWires smoke-tests that the overdue command
// resolves at runtime and renders useful --help output. Catches wiring
// regressions (missing AddCommand, panicking RunE on --help, etc.) before
// review. Keep this smoke test when adding behavior-specific cases.
func TestNovelOverdueHelpWires(t *testing.T) {
	cmd := RootCmd()
	cmd.SetArgs([]string{"overdue", "--help"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("overdue --help error = %v (novel command not wired correctly?)", err)
	}
	help := out.String()
	for _, want := range []string{"Usage:", "overdue"} {
		if !strings.Contains(help, want) {
			t.Fatalf("overdue --help missing %q in output:\n%s", want, help)
		}
	}
}

func TestZammadOverdueScoreUsesFractionalDays(t *testing.T) {
	for _, tt := range []struct {
		hours  int
		weight float64
		want   float64
	}{
		{hours: 6, weight: 1, want: 0.25},
		{hours: 12, weight: 3, want: 1.5},
		{hours: 24, weight: 0.5, want: 0.5},
	} {
		if got := zammadOverdueScore(tt.hours, tt.weight); got != tt.want {
			t.Errorf("zammadOverdueScore(%d, %v) = %v, want %v", tt.hours, tt.weight, got, tt.want)
		}
	}
}
