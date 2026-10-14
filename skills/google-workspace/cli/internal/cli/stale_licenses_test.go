// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// cli-printing-press: novel-scaffold-test
// Novel command scaffold tests. Keep the wiring smoke test and add behavior cases as needed.

package cli

import (
	"bytes"
	"strings"
	"testing"

	"google-workspace-pp-cli/internal/cliutil/testenv"
)

// TestNovelStaleLicensesHelpWires smoke-tests that the stale-licenses command
// resolves at runtime and renders useful --help output. Catches wiring
// regressions (missing AddCommand, panicking RunE on --help, etc.) before
// review. Keep this smoke test when adding behavior-specific cases.
func TestNovelStaleLicensesHelpWires(t *testing.T) {
	testenv.Isolate(t)
	cmd := RootCmd()
	cmd.SetArgs([]string{"stale-licenses", "--help"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("stale-licenses --help error = %v (novel command not wired correctly?)", err)
	}
	help := out.String()
	for _, want := range []string{"Usage:", "stale-licenses"} {
		if !strings.Contains(help, want) {
			t.Fatalf("stale-licenses --help missing %q in output:\n%s", want, help)
		}
	}
}

// TestFreeSKUsAreNotWaste: Nonprofits and Education Fundamentals seats cost
// nothing, so a suspended account holding one is not licence waste.
func TestFreeSKUsAreNotWaste(t *testing.T) {
	now := testNow
	users := []dirUser{{PrimaryEmail: "gone@example.org", Suspended: true}}
	free := []licenseAssignment{{UserID: "gone@example.org", ProductID: "Google-Apps", SkuID: "Google-Apps-For-Nonprofits", SkuName: "Google Workspace for Nonprofits"}}
	if got := staleLicensesFor("acme", free, users, now, 90, 0); len(got) != 0 {
		t.Fatalf("a free Nonprofits seat must not be reported as waste: %+v", got)
	}
	paid := []licenseAssignment{{UserID: "gone@example.org", ProductID: "Google-Apps", SkuID: "1010020028", SkuName: "Google Workspace Business Standard"}}
	if got := staleLicensesFor("acme", paid, users, now, 90, 0); len(got) != 1 {
		t.Fatalf("a paid seat on a suspended account is waste: %+v", got)
	}
}
