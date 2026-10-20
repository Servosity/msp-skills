// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"eset-protect-pp-cli/internal/cliutil/testenv"
)

// TestImportErrorsAreEscaped (handfixes.json: stderr-errors-escaped): import's
// failed-record warnings wrote the API error straight to os.Stderr, so a 400
// echoing a name with C1 or bidi controls reached the terminal raw.
func TestImportErrorsAreEscaped(t *testing.T) {
	testenv.Isolate(t)
	respondClearHarness(t)
	respondFakeESET(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"code":3,"message":"bad name PC\u009d0;evil\u009c‮exe"}`)
	})
	in := filepath.Join(t.TempDir(), "in.jsonl")
	if err := os.WriteFile(in, []byte(`{"displayName":"x"}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, errOut, _ := captureExecuteAll(t, "import", "groups", "--input", in)
	if !strings.Contains(string(errOut), "failed to import record") {
		t.Fatalf("no failed-record warning: %q", errOut)
	}
	if what, leak := terminalLeak(errOut); leak {
		t.Fatalf("the import warning leaked %s to stderr: %q", what, errOut)
	}
}
