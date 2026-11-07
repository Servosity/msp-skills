// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package salesbuildrblackbox

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// Codex r1 P2 export-hangs-after-api-error: export --output writes through the
// withholding file sink (a filter pipe). An API error after the file is open
// must still close it, or the process waits for that pipe forever on exit.
func TestExportOutputReturnsAfterAnAPIError(t *testing.T) {
	sv := echoServer(t)
	e := newEnv(t, sv, credEnvToken)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	// The fake answers this opportunity read 401.
	cmd := exec.CommandContext(ctx, bbCLI, "export", "opportunity", bbDenied, "--output", filepath.Join(e.home, "opportunity.jsonl"))
	cmd.Env = e.env
	cmd.Dir = e.home
	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("export --output did not return after Salesbuildr answered 401:\n%s", out)
	}
	if err == nil {
		t.Fatalf("export --output succeeded on a 401:\n%s", out)
	}
	// The read reached the fake, so the error above is the API's 401 after the
	// file opened, not a usage error before it.
	sv.mu.Lock()
	defer sv.mu.Unlock()
	want := "GET " + bbPrefix + "/opportunity/" + bbDenied
	if len(sv.methods) != 1 || sv.methods[0] != want {
		t.Fatalf("want the one %s, got %v:\n%s", want, sv.methods, out)
	}
}
