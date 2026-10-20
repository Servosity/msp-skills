// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// cli-printing-press: novel-scaffold-test
// Novel command scaffold tests. Keep the wiring smoke test and add behavior cases as needed.

package cli

import (
	"bytes"
	"strings"
	"testing"

	"eset-protect-pp-cli/internal/cliutil/testenv"
)

// TestNovelRespondHelpWires smoke-tests that the respond command
// resolves at runtime and renders useful --help output. Catches wiring
// regressions (missing AddCommand, panicking RunE on --help, etc.) before
// review. Keep this smoke test when adding behavior-specific cases.
func TestNovelRespondHelpWires(t *testing.T) {
	testenv.Isolate(t)
	cmd := RootCmd()
	cmd.SetArgs([]string{"respond", "--help"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("respond --help error = %v (novel command not wired correctly?)", err)
	}
	help := out.String()
	for _, want := range []string{"Usage:", "respond"} {
		if !strings.Contains(help, want) {
			t.Fatalf("respond --help missing %q in output:\n%s", want, help)
		}
	}
}

// TestNovelRespondSubcommandsWire checks each subcommand's help, the
// manifest's Use/Do NOT text, the isolation warning, and the annotations the
// MCP walker and the dogfood matrix read.
func TestNovelRespondSubcommandsWire(t *testing.T) {
	testenv.Isolate(t)
	for _, sub := range []string{"scan", "isolate", "release"} {
		cmd := RootCmd()
		cmd.SetArgs([]string{"respond", sub, "--help"})
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		if err := cmd.Execute(); err != nil {
			t.Fatalf("respond %s --help: %v", sub, err)
		}
		help := out.String()
		for _, want := range []string{"Use this command to scan, isolate or release one named device", "Do NOT use it for scheduled or arbitrary task actions; use 'device-tasks create' instead.", "--wait", "--db", "only previews"} {
			if !strings.Contains(help, want) {
				t.Errorf("respond %s --help missing %q", sub, want)
			}
		}
		if sub == "isolate" && !strings.Contains(help, "except to ESET management") {
			t.Errorf("respond isolate --help must warn that the device loses network access")
		}
		if sub != "scan" && strings.Contains(help, "--customer string") {
			t.Errorf("respond %s must not list --customer as a flag", sub)
		}
	}

	root := RootCmd()
	parent, _, err := root.Find([]string{"respond"})
	if err != nil || parent.Name() != "respond" {
		t.Fatalf("respond not wired: %v", err)
	}
	if parent.Annotations["pp:data-source"] != "live" || parent.Annotations["pp:novel-scaffold"] != "" || parent.Annotations["pp:typed-exit-codes"] != "0,2" {
		t.Fatalf("parent annotations: %v", parent.Annotations)
	}
	for _, sub := range []string{"scan", "isolate", "release"} {
		c, _, err := root.Find([]string{"respond", sub})
		if err != nil || c.Name() != sub {
			t.Fatalf("respond %s not wired: %v", sub, err)
		}
		a := c.Annotations
		if a["mcp:read-only"] == "true" || a["pp:data-source"] != "live" || a["pp:preview-happy-path"] != "true" || a["pp:happy-args"] != "device=example-value" {
			t.Errorf("respond %s annotations: %v", sub, a)
		}
	}
}

func TestNovelRespondBareParentIsUsageErrorInJSON(t *testing.T) {
	testenv.Isolate(t)
	cmd := RootCmd()
	cmd.SetArgs([]string{"respond", "--json"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	err := cmd.Execute()
	if err == nil || ExitCode(err) != 2 || !strings.Contains(out.String(), "valid_subcommands") {
		t.Fatalf("bare 'respond --json' must be a structured usage error: code=%d %v\n%s", ExitCode(err), err, out.String())
	}
}
