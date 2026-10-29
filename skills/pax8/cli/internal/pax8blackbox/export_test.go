// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package pax8blackbox

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// Codex r1 P2 export-hangs-after-api-error: an API error after export --output
// opened its file must still return.
func TestExportOutputReturnsAfterAnAPIError(t *testing.T) {
	sv := echoServer(t)
	e := newEnv(t, sv, true).store(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bbCLI, "export", "security", "--output", filepath.Join(e.home, "alerts.jsonl"))
	cmd.Env = e.env
	cmd.Dir = e.home
	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("export --output did not return after Graph answered 401:\n%s", out)
	}
	if err == nil {
		t.Fatalf("export --output succeeded on a 401:\n%s", out)
	}
}
