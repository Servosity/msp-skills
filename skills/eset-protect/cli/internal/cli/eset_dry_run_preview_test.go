// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"net/http"
	"testing"

	"eset-protect-pp-cli/internal/cliutil/testenv"
)

// TestDryRunPreviewIsEscaped (handfixes.json: dry-run-preview-escaped): the
// client printed dry-run request previews straight to os.Stderr, so a display
// name carrying C1 controls (U+009D OSC, U+009C ST) or a bidi override reached
// the approval preview raw. Previews now go through the escaping writer.
func TestDryRunPreviewIsEscaped(t *testing.T) {
	testenv.Isolate(t)
	respondClearHarness(t)
	respondFakeESET(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("a dry run sent %s %s", r.Method, r.URL.Path)
	})
	name := "PC\u009d0;evil\u009c‮exe.txt"
	_, errOut, err := captureExecuteAll(t, "devices", "rename", "00000000-0000-0000-0000-000000000001", "--display-name", name, "--dry-run")
	if err != nil {
		t.Fatalf("dry run: %v\n%s", err, errOut)
	}
	if what, leak := terminalLeak(errOut); leak {
		t.Fatalf("the dry-run preview leaked %s to stderr: %q", what, errOut)
	}
}
