// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// cli-printing-press: novel-scaffold-test
// Novel command scaffold tests. Keep the wiring smoke test and add behavior cases as needed.

package cli

import (
	"bytes"
	"strings"
	"testing"

	"salesbuildr-pp-cli/internal/cliutil/testenv"
)

// TestNovelOpportunityMrrForecastHelpWires smoke-tests that the opportunity mrr-forecast command
// resolves at runtime and renders useful --help output. Catches wiring
// regressions (missing AddCommand, panicking RunE on --help, etc.) before
// review. Keep this smoke test when adding behavior-specific cases.
func TestNovelOpportunityMrrForecastHelpWires(t *testing.T) {
	testenv.Isolate(t)
	cmd := RootCmd()
	cmd.SetArgs([]string{"opportunity", "mrr-forecast", "--help"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("opportunity mrr-forecast --help error = %v (novel command not wired correctly?)", err)
	}
	help := out.String()
	for _, want := range []string{"Usage:", "mrr-forecast"} {
		if !strings.Contains(help, want) {
			t.Fatalf("opportunity mrr-forecast --help missing %q in output:\n%s", want, help)
		}
	}
}
