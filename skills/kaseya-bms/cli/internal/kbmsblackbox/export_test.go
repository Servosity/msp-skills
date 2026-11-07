// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package kbmsblackbox

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// Codex r1 P2 export-hangs-after-api-error: export --output writes through the
// withholding file sink (a filter pipe). An API error must still close it, or
// the process waits for that pipe forever on exit. The fake answers the
// note templates lookup (the list `export servicedesk` reads) 401; with the
// login the export mints first, so the token path is covered too.
func TestExportOutputReturnsAfterAnAPIError(t *testing.T) {
	t.Parallel() // its own server, home and env: independent of every other test
	for _, mode := range []credMode{credStored, credLogin} {
		t.Run(mode.String(), func(t *testing.T) {
			sv := echoServer(t)
			e := newEnv(t, sv, mode)
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, bbCLI, "export", "servicedesk", "--output", filepath.Join(e.home, "templates.jsonl"))
			cmd.Env = e.env
			cmd.Dir = e.home
			out, err := cmd.CombinedOutput()
			if ctx.Err() != nil {
				t.Fatalf("export --output did not return after Kaseya BMS answered 401:\n%s", out)
			}
			if err == nil {
				t.Fatalf("export --output succeeded on a 401:\n%s", out)
			}
			// The 401 really came from the API, not a usage error before any request.
			var asked bool
			methods, _, _, _ := sv.counts()
			for _, m := range methods {
				asked = asked || m == "GET /v2/servicedesk/templates/notes/lookup"
			}
			if !asked {
				t.Fatalf("export never asked for the note templates list (requests %v):\n%s", methods, out)
			}
			assertNoEcho(t, "export output", string(out))
		})
	}
}
