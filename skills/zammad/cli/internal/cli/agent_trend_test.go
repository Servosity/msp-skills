// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// cli-printing-press: novel-scaffold-test
// Novel command scaffold tests. Keep the wiring smoke test and add behavior cases as needed.

package cli

import (
	"bytes"
	"strings"
	"testing"
)

// TestNovelAgentTrendHelpWires smoke-tests that the agent-trend command
// resolves at runtime and renders useful --help output. Catches wiring
// regressions (missing AddCommand, panicking RunE on --help, etc.) before
// review. Keep this smoke test when adding behavior-specific cases.
func TestNovelAgentTrendHelpWires(t *testing.T) {
	cmd := RootCmd()
	cmd.SetArgs([]string{"agent-trend", "--help"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("agent-trend --help error = %v (novel command not wired correctly?)", err)
	}
	help := out.String()
	for _, want := range []string{"Usage:", "agent-trend", "current owner"} {
		if !strings.Contains(help, want) {
			t.Fatalf("agent-trend --help missing %q in output:\n%s", want, help)
		}
	}
}

func TestAgentTrendDirectionUsesCurrentNetFlow(t *testing.T) {
	for _, tt := range []struct {
		net  int
		want string
	}{
		{net: 4, want: "growing"},
		{net: -2, want: "shrinking"},
		{net: 0, want: "flat"},
	} {
		if got := agentTrendDirection(tt.net); got != tt.want {
			t.Errorf("agentTrendDirection(%d) = %q, want %q", tt.net, got, tt.want)
		}
	}
}
