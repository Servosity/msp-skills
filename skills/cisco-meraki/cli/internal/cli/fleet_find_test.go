// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// cli-printing-press: novel-scaffold-test
// Novel command scaffold tests. Keep the wiring smoke test and add behavior cases as needed.

package cli

import (
	"bytes"
	"strings"
	"testing"

	"cisco-meraki-pp-cli/internal/cliutil/testenv"
)

// TestNovelFleetFindHelpWires smoke-tests that the fleet find command
// resolves at runtime and renders useful --help output. Catches wiring
// regressions (missing AddCommand, panicking RunE on --help, etc.) before
// review. Keep this smoke test when adding behavior-specific cases.
func TestNovelFleetFindHelpWires(t *testing.T) {
	testenv.Isolate(t)
	cmd := RootCmd()
	cmd.SetArgs([]string{"fleet", "find", "--help"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("fleet find --help error = %v (novel command not wired correctly?)", err)
	}
	help := out.String()
	for _, want := range []string{"Usage:", "find"} {
		if !strings.Contains(help, want) {
			t.Fatalf("fleet find --help missing %q in output:\n%s", want, help)
		}
	}
}

// TestFleetFindMatchesIPAddressesNotText: one address, several spellings
// (Codex round 24).
func TestFleetFindMatchesIPAddressesNotText(t *testing.T) {
	r := findRecord{LanIP: "192.0.2.1", Uplinks: []struct {
		IP       string `json:"ip"`
		PublicIP string `json:"publicIp"`
	}{{PublicIP: "2001:DB8:0:0::1"}}}
	for _, q := range []string{"::ffff:192.0.2.1", "192.0.2.1", "2001:db8::1"} {
		kind, value := classifyIdentifier(q)
		if kind != "ip" || !matchRecord(r, kind, value) {
			t.Fatalf("%q (%s %q) did not match", q, kind, value)
		}
	}
	if kind, value := classifyIdentifier("192.0.2.2"); matchRecord(r, kind, value) {
		t.Fatal("a different address matched")
	}
}
