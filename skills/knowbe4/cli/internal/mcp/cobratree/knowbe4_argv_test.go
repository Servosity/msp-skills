// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cobratree

import (
	"context"
	"errors"
	"strings"
	"testing"

	"knowbe4-pp-cli/internal/credkit"
)

// Class held-value-crosses-process-boundary (Codex r2): the child CLI never
// receives an argv that repeats a value this MCP server holds, whatever form
// the tool arguments took before the mirror formatted them (a JSON number the
// mirror prints as float64 included): RunCLICommand starts the child through
// the credential kit (credkit.RunCommand), which refuses it.
func TestChildArgvNeverRepeatsAHeldValue(t *testing.T) {
	if runInChild(t) {
		return
	}
	const session = "10000000000000000000000000000000"
	credkit.Hold("stdin", session)
	argv := append([]string{"teach", "--query=find application"}, cliArgsFromMCP(map[string]any{"notes": float64(1e31)}, nil)...)
	if !strings.Contains(strings.Join(argv, " "), session) {
		t.Fatalf("test setup: the mirror's argv %v does not carry the held value", argv)
	}
	_, err := RunCLICommand(context.Background(), "/nonexistent/knowbe4-cli", argv)
	if !errors.Is(err, credkit.ErrWithheld) {
		t.Fatalf("RunCLICommand with a held value in argv: want the refusal, got %v", err)
	}
	if strings.Contains(err.Error(), session) {
		t.Fatalf("the refusal repeats the held value: %v", err)
	}
}
