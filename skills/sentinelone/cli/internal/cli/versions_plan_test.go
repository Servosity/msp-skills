// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// cli-printing-press: novel-scaffold-test
// Novel command scaffold tests. Keep the wiring smoke test and add behavior cases as needed.

package cli

import (
	"bytes"
	"strings"
	"testing"

	"sentinelone-pp-cli/internal/cliutil/testenv"
)

// TestNovelVersionsPlanHelpWires smoke-tests that the versions plan command
// resolves at runtime and renders useful --help output. Catches wiring
// regressions (missing AddCommand, panicking RunE on --help, etc.) before
// review. Keep this smoke test when adding behavior-specific cases.
func TestNovelVersionsPlanHelpWires(t *testing.T) {
	testenv.Isolate(t)
	cmd := RootCmd()
	cmd.SetArgs([]string{"versions", "plan", "--help"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("versions plan --help error = %v (novel command not wired correctly?)", err)
	}
	help := out.String()
	for _, want := range []string{"Usage:", "plan"} {
		if !strings.Contains(help, want) {
			t.Fatalf("versions plan --help missing %q in output:\n%s", want, help)
		}
	}
}

// TestVersionsPlanTargetsEachOSSeparately: Windows and macOS agents ship
// separate packages, so the default target is the newest version per OS and a
// macOS agent is never planned toward a Windows version number.
func TestVersionsPlanTargetsEachOSSeparately(t *testing.T) {
	agents := []map[string]any{
		agentFix("w1", "WIN-NEW", "SiteA", "24.1.2.6", map[string]any{"osType": "windows"}),
		agentFix("w2", "WIN-OLD", "SiteA", "23.4.1.1", map[string]any{"osType": "windows"}),
		agentFix("w3", "WIN-OFF", "SiteA", "23.4.1.1", map[string]any{"osType": "windows", "isActive": false}),
		agentFix("m1", "MAC-NEW", "SiteA", "23.3.1.5", map[string]any{"osType": "macos"}),
		agentFix("m2", "MAC-OLD", "SiteB", "23.2.0.9", map[string]any{"osType": "macos"}),
		agentFix("x1", "NOVER", "SiteB", "", map[string]any{"osType": "linux"}),
	}
	db := newSeededStore(t, agents, nil)
	out := runJSON(t, newNovelVersionsPlanCmd(jsonFlags()), "--db", db)
	targets := out["target_by_os"].(map[string]any)
	if targets["windows"] != "24.1.2.6" || targets["macos"] != "23.3.1.5" {
		t.Fatalf("target_by_os = %v, want windows 24.1.2.6 and macos 23.3.1.5", targets)
	}
	if out["reachable_total"].(float64) != 2 || out["offline_total"].(float64) != 1 || out["unknown_version"].(float64) != 1 {
		t.Fatalf("totals = reachable %v offline %v unknown %v, want 2/1/1", out["reachable_total"], out["offline_total"], out["unknown_version"])
	}
	for _, g := range out["groups"].([]any) {
		m := g.(map[string]any)
		for _, a := range m["reachable"].([]any) {
			name := a.(map[string]any)["computer_name"]
			if name == "MAC-NEW" || name == "WIN-NEW" {
				t.Fatalf("%v is at its OS's newest version and must not be planned", name)
			}
		}
	}
}

func TestVersionsPlanRefusesAnUnscopedOrMalformedTarget(t *testing.T) {
	db := newSeededStore(t, []map[string]any{agentFix("w1", "W", "S", "24.1", map[string]any{"osType": "windows"})}, nil)
	for _, args := range [][]string{
		{"--target", "24.1.2.6"},                  // no --os: would plan macOS agents toward a Windows version
		{"--os", "windows", "--target", "24.1.x"}, // not a version
		{"--os", "solaris"},                       // no such OS in the store
	} {
		cmd := newNovelVersionsPlanCmd(jsonFlags())
		cmd.SetArgs(append([]string{"--db", db}, args...))
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})
		if err := cmd.Execute(); err == nil {
			t.Fatalf("%v must be refused", args)
		}
	}
}
