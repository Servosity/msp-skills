// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// cli-printing-press: novel-scaffold-test
// Novel command scaffold tests. Keep the wiring smoke test and add behavior cases as needed.

package cli

import (
	"bytes"
	"strings"
	"testing"

	"servosity-msp-pp-cli/internal/cliutil/testenv"
)

// TestNovelRetentionAuditHelpWires smoke-tests that the retention-audit command
// resolves at runtime and renders useful --help output. Catches wiring
// regressions (missing AddCommand, panicking RunE on --help, etc.) before
// review. Keep this smoke test when adding behavior-specific cases.
func TestNovelRetentionAuditHelpWires(t *testing.T) {
	testenv.Isolate(t)
	cmd := RootCmd()
	cmd.SetArgs([]string{"retention-audit", "--help"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("retention-audit --help error = %v (novel command not wired correctly?)", err)
	}
	help := out.String()
	for _, want := range []string{"Usage:", "retention-audit"} {
		if !strings.Contains(help, want) {
			t.Fatalf("retention-audit --help missing %q in output:\n%s", want, help)
		}
	}
}

func TestRetentionReasons(t *testing.T) {
	eligible, ineligible := true, false
	min7 := retentionRank("7YEAR")
	cases := []struct {
		retention string
		guarantee *bool
		minRank   int
		check     bool
		want      int
	}{
		{"10YEAR", &eligible, min7, true, 0},
		{"7year", &eligible, min7, true, 0},
		{"4YEAR", &eligible, min7, true, 1},
		{"", &eligible, min7, true, 1},
		{"FOREVER", &eligible, min7, true, 1},
		{"10YEAR", &ineligible, min7, true, 1},
		{"10YEAR", nil, min7, true, 1},
		{"1YEAR", &ineligible, min7, true, 2},
		{"1YEAR", &ineligible, -1, true, 1},
		{"1YEAR", &ineligible, min7, false, 1},
		{"1YEAR", &ineligible, -1, false, 0},
	}
	for _, c := range cases {
		if got := retentionReasons(c.retention, c.guarantee, c.minRank, c.check); len(got) != c.want {
			t.Errorf("retentionReasons(%q, %v, %d, %v) = %q, want %d reasons", c.retention, c.guarantee, c.minRank, c.check, got, c.want)
		}
	}
}

func TestRetentionAuditRejectsAnEmptyAudit(t *testing.T) {
	testenv.Isolate(t)
	cmd := RootCmd()
	cmd.SetArgs([]string{"retention-audit", "--ignore-guarantee"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err == nil {
		t.Fatal("retention-audit --ignore-guarantee without --min must refuse: there is nothing to audit")
	}
}
