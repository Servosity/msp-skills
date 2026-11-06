// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cobratree

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"

	"kaseya-bms-pp-cli/internal/credkit"
)

// Class held-value-crosses-process-boundary (Codex r2, fixed after the stop
// rule): the child CLI never receives an argv that repeats a value this MCP
// server holds, whatever form the tool arguments took before the mirror
// formatted them (a JSON number the mirror prints as float64 included).
//
// The held value stays held for the rest of the process (the kit has no
// release), and this digit-only one would refuse every later child in the
// package, so the check runs in a child test process of its own.
func TestChildArgvNeverRepeatsAHeldValue(t *testing.T) {
	if os.Getenv("KBMS_ARGV_HELD_CHILD") != "1" {
		cmd := exec.Command(os.Args[0], "-test.run=^TestChildArgvNeverRepeatsAHeldValue$", "-test.count=1", "-test.v")
		cmd.Env = append(os.Environ(), "KBMS_ARGV_HELD_CHILD=1")
		out, err := cmd.CombinedOutput()
		if err != nil || !strings.Contains(string(out), "--- PASS: TestChildArgvNeverRepeatsAHeldValue") {
			t.Fatalf("the held-argv check failed in its own process (err=%v):\n%s", err, out)
		}
		return
	}
	const session = "10000000000000000000000000000000"
	credkit.Hold("test:held", session)
	argv := append([]string{"teach", "--query=find application"}, cliArgsFromMCP(map[string]any{"notes": float64(1e31)}, nil)...)
	if !strings.Contains(strings.Join(argv, " "), session) {
		t.Fatalf("test setup: the mirror's argv %v does not carry the held value", argv)
	}
	_, err := RunCLICommand(context.Background(), "/nonexistent/kaseya-bms-cli", argv)
	if !errors.Is(err, credkit.ErrWithheld) {
		t.Fatalf("RunCLICommand with a held value in argv: want the refusal, got %v", err)
	}
	if strings.Contains(err.Error(), session) {
		t.Fatalf("the refusal repeats the held value: %v", err)
	}
}
