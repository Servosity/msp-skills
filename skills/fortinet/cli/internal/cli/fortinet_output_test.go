// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"strings"
	"testing"
)

// runCLIStreams runs the root command and returns stdout and stderr separately.
func runCLIStreams(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	cmd := RootCmd()
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), errOut.String(), err
}

// An unreadable fleet must never print the all-clear line in the human table.
func TestPoliciesHygieneHumanNoAllClearWhenUnreadable(t *testing.T) {
	f := newFleetFixture(t)
	out, _, err := runCLIStreams(t, "policies", "hygiene", "--devices", f.devices, "--device", "b", "--human-friendly")
	if err != nil {
		t.Fatalf("policies hygiene: %v\n%s", err, out)
	}
	if strings.Contains(out, "No risky or unused policies found.") {
		t.Errorf("all-clear printed for an unreadable fleet:\n%s", out)
	}
	if !strings.Contains(out, "No policies evaluated") || !strings.Contains(out, "UNKNOWN") {
		t.Errorf("unreadable fleet not reported as unevaluated:\n%s", out)
	}
}

// fleet audit --new on an unchanged fleet with open findings must not read
// as "No findings.".
func TestFleetAuditNewHumanKeepsOpenFindingsVisible(t *testing.T) {
	f := newFleetFixture(t)
	if _, _, err := runCLIStreams(t, "fleet", "audit", "--devices", f.devices, "--device", "b", "--json"); err != nil {
		t.Fatalf("first run: %v", err)
	}
	out, _, err := runCLIStreams(t, "fleet", "audit", "--devices", f.devices, "--device", "b", "--new", "--human-friendly")
	if err != nil {
		t.Fatalf("fleet audit --new: %v\n%s", err, out)
	}
	if strings.Contains(out, "No findings.") {
		t.Errorf("--new summary reads as all-clear:\n%s", out)
	}
	if !strings.Contains(out, "No new findings; ") || !strings.Contains(out, "finding(s) from the previous run are still open") {
		t.Errorf("--new summary does not mention the open finding:\n%s", out)
	}
}

// Explanations an agent needs must carry a prefix the MCP bridge forwards
// (cobratree stderrHintLines keeps only hint:/warning: lines).
func TestFleetChangesNoBackupsHintIsForwardable(t *testing.T) {
	newFleetFixture(t)
	out, errOut, err := runCLIStreams(t, "fleet", "changes", "--dir", t.TempDir(), "--json")
	if err != nil {
		t.Fatalf("fleet changes: %v\n%s", err, out)
	}
	if !strings.HasPrefix(strings.TrimSpace(errOut), "hint: no backups under ") || !strings.Contains(errOut, "fleet backup") {
		t.Errorf("no-backups explanation is not a forwardable hint line: %q", errOut)
	}
}
