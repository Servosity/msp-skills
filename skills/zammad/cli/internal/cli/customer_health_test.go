// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// cli-printing-press: novel-scaffold-test
// Novel command scaffold tests. Keep the wiring smoke test and add behavior cases as needed.

package cli

import (
	"bytes"
	"strings"
	"testing"
)

// TestNovelCustomerHealthHelpWires smoke-tests that the customer-health command
// resolves at runtime and renders useful --help output. Catches wiring
// regressions (missing AddCommand, panicking RunE on --help, etc.) before
// review. Keep this smoke test when adding behavior-specific cases.
func TestNovelCustomerHealthHelpWires(t *testing.T) {
	cmd := RootCmd()
	cmd.SetArgs([]string{"customer-health", "--help"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("customer-health --help error = %v (novel command not wired correctly?)", err)
	}
	help := out.String()
	for _, want := range []string{"Usage:", "customer-health", "--include-unassigned"} {
		if !strings.Contains(help, want) {
			t.Fatalf("customer-health --help missing %q in output:\n%s", want, help)
		}
	}
}

func TestAverageZammadAgeDaysUsesParsedAgeDenominator(t *testing.T) {
	for _, tt := range []struct {
		name        string
		total       int
		parsedCount int
		want        int
	}{
		{name: "unknown dates excluded", total: 30, parsedCount: 2, want: 15},
		{name: "one parsed among many open", total: 30, parsedCount: 1, want: 30},
		{name: "no parsed dates", total: 0, parsedCount: 0, want: 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := averageZammadAgeDays(tt.total, tt.parsedCount); got != tt.want {
				t.Fatalf("averageZammadAgeDays(%d, %d) = %d, want %d", tt.total, tt.parsedCount, got, tt.want)
			}
		})
	}
}
