// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cobratree

import (
	"context"
	"strings"
	"testing"

	"pax8-pp-cli/internal/cliutil"
)

// Class held-value-crosses-process-boundary (Codex r2, fixed after the stop
// rule): the child CLI never receives an argv that repeats a value this MCP
// server holds, whatever form the tool arguments took before the mirror
// formatted them (a JSON number the mirror prints as float64 included).
func TestChildArgvNeverRepeatsAHeldValue(t *testing.T) {
	const session = "10000000000000000000000000000000"
	cliutil.FunnelHold(session)
	argv := append([]string{"teach", "--query=find application"}, cliArgsFromMCP(map[string]any{"notes": float64(1e31)}, nil)...)
	if !strings.Contains(strings.Join(argv, " "), session) {
		t.Fatalf("test setup: the mirror's argv %v does not carry the held value", argv)
	}
	_, err := RunCLICommand(context.Background(), "/nonexistent/pax8-cli", argv)
	if err == nil || !strings.Contains(err.Error(), "repeats a credential") {
		t.Fatalf("RunCLICommand with a held value in argv: want the refusal, got %v", err)
	}
	if strings.Contains(err.Error(), session) {
		t.Fatalf("the refusal repeats the held value: %v", err)
	}
}
