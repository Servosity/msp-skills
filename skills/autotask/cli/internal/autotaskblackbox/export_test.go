// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package autotaskblackbox

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Codex r1 P2 export-hangs-after-api-error: export --output writes through the
// kit's file sink (credkit.NewWriter). An API error after the file is open
// must still close it and return.
func TestExportOutputReturnsAfterAnAPIError(t *testing.T) {
	t.Parallel()
	sv := echoServer(t)
	e := newEnv(t, sv, credEnv)
	// A hang never returns; a slow run under the parallel suite takes 7 to 30 s
	// (each store open costs seconds with a credential held: kit finding 2).
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	// The fake answers every invoice path 401.
	cmd := exec.CommandContext(ctx, bbCLI, "export", "invoices", "--output", filepath.Join(e.home, "invoices.jsonl"))
	cmd.Env = e.env
	cmd.Dir = e.home
	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("export --output did not return after Autotask answered 401:\n%s", out)
	}
	if err == nil {
		t.Fatalf("export --output succeeded on a 401:\n%s", out)
	}
	// The export reached the API (an argument error would fail fast too).
	sv.mu.Lock()
	defer sv.mu.Unlock()
	var asked bool
	for _, m := range sv.methods {
		asked = asked || strings.Contains(m, " "+apiBase+"/Invoices")
	}
	if !asked {
		t.Fatalf("export never asked the API for the invoices; the test did not run:\n%s", out)
	}
}
