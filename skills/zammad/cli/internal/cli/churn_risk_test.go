// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// cli-printing-press: novel-scaffold-test
// Novel command scaffold tests. Keep the wiring smoke test and add behavior cases as needed.

package cli

import (
	"bytes"
	"strings"
	"testing"
)

// TestNovelChurnRiskHelpWires smoke-tests that the churn-risk command
// resolves at runtime and renders useful --help output. Catches wiring
// regressions (missing AddCommand, panicking RunE on --help, etc.) before
// review. Keep this smoke test when adding behavior-specific cases.
func TestNovelChurnRiskHelpWires(t *testing.T) {
	cmd := RootCmd()
	cmd.SetArgs([]string{"churn-risk", "--help"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("churn-risk --help error = %v (novel command not wired correctly?)", err)
	}
	help := out.String()
	for _, want := range []string{"Usage:", "churn-risk", "--include-unassigned"} {
		if !strings.Contains(help, want) {
			t.Fatalf("churn-risk --help missing %q in output:\n%s", want, help)
		}
	}
	if strings.Contains(strings.ToLower(help), "trend") {
		t.Fatalf("churn-risk --help still claims to compute a trend:\n%s", help)
	}
}

func TestChurnRiskClassificationRequiresCorroborationForHigh(t *testing.T) {
	for _, tt := range []struct {
		name string
		row  churnRiskRow
		want string
	}{
		{name: "backlog alone capped at watch", row: churnRiskRow{Score: 20, Backlog: 20}, want: "watch"},
		{name: "overdue corroborates high", row: churnRiskRow{Score: 10, Backlog: 8, Overdue: 1}, want: "high"},
		{name: "negative sentiment corroborates high", row: churnRiskRow{Score: 10, Backlog: 7, NegativeTickets: 1}, want: "high"},
		{name: "small score stays low", row: churnRiskRow{Score: 3, Overdue: 1}, want: "low"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := churnRiskClassification(tt.row); got != tt.want {
				t.Fatalf("churnRiskClassification(%+v) = %q, want %q", tt.row, got, tt.want)
			}
		})
	}
}
