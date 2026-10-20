// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/dnsfilter/handfixes.json
// under the "mcp-failure-keeps-stdout-remedy" entry.

package cobratree

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const resumedSyncWarning = `{"event":"sync_warning","resource":"user-agents","reason":"resumed_from_cursor","message":"the walk resumed from a saved cursor; run 'dnsfilter-cli sync --full --resources user-agents' to complete it"}`

// TestFailedStdoutDetailPicksRemedyLines: warning/hint/error events and
// lines are kept, data lines dropped; with none, the last lines are kept.
func TestFailedStdoutDetailPicksRemedyLines(t *testing.T) {
	stdout := `{"event":"sync_start","resource":"user-agents"}` + "\n" + `{"id":"a1","hostname":"PC-1"}` + "\n" + resumedSyncWarning + "\nhint: run with --full\n" + `{"event":"sync_summary","warned":1}` + "\n"
	got := failedStdoutDetail(stdout)
	if !strings.Contains(got, "sync --full --resources user-agents") || !strings.Contains(got, "hint: run with --full") {
		t.Fatalf("remedy lines dropped: %q", got)
	}
	if strings.Contains(got, "PC-1") || strings.Contains(got, "sync_start") {
		t.Fatalf("data lines kept: %q", got)
	}
	var rows []string
	for i := 1; i <= 30; i++ {
		rows = append(rows, fmt.Sprintf("row %d", i))
	}
	tail := failedStdoutDetail(strings.Join(rows, "\n"))
	if !strings.HasPrefix(tail, "row 11\n") || !strings.HasSuffix(tail, "row 30") {
		t.Fatalf("tail of plain stdout = %q", tail)
	}
	big := failedStdoutDetail(strings.Repeat("warning: "+strings.Repeat("x", 100)+"\n", 200))
	if len(big) > failedStdoutDetailBytes {
		t.Fatalf("detail not bounded: %d bytes", len(big))
	}
}

// TestRunCLICommandFailureKeepsStdoutRemedy: a command that exits non-zero
// with a generic stderr line and its remedy on stdout (sync on a resumed
// walk) reports the remedy in the error.
func TestRunCLICommandFailureKeepsStdoutRemedy(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell helper; failedStdoutDetail is covered portably above")
	}
	dir := t.TempDir()
	data := filepath.Join(dir, "stdout.txt")
	if err := os.WriteFile(data, []byte(`{"event":"sync_start","resource":"user-agents"}`+"\n"+resumedSyncWarning+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(dir, "helper.sh")
	body := "#!/bin/sh\ncat '" + data + "'\necho 'Error: 1 resource(s) completed with warnings but no successful syncs' >&2\nexit 1\n"
	if err := os.WriteFile(bin, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := RunCLICommand(context.Background(), bin, nil)
	if err == nil {
		t.Fatal("helper succeeded unexpectedly")
	}
	msg := err.Error()
	if !strings.Contains(msg, "completed with warnings but no successful syncs") {
		t.Fatalf("stderr dropped: %s", msg)
	}
	if !strings.Contains(msg, "sync --full --resources user-agents") {
		t.Fatalf("the stdout remedy was discarded on a non-zero exit: %s", msg)
	}
}
