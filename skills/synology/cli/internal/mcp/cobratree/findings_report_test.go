// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/synology/handfixes.json
// under the "mcp-findings-keep-report" entry.

package cobratree

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
)

// TestFindingsHelperProcess stands in for `synology-cli fleet status --agent`
// finding a problem: rows on stdout, the count on stderr, exit 6. It does
// nothing unless the parent test asked for it.
func TestFindingsHelperProcess(t *testing.T) {
	if os.Getenv("SYNOLOGY_SWEEP_FINDINGS_HELPER") != "1" {
		return
	}
	fmt.Fprint(os.Stdout, `{"results":[{"nas":"acme","state":"critical","sync_error":"login refused"}]}`)
	fmt.Fprint(os.Stderr, "Error: 1 row(s) need attention (exit 6; --no-fail exits 0)\n")
	os.Exit(6)
}

// TestShellOutFindingsKeepRows: the fleet and abb report commands exit 6 when
// they found something; RunCLICommand quoted only stderr, so an MCP caller
// got "1 row(s) need attention" without the rows.
func TestShellOutFindingsKeepRows(t *testing.T) {
	t.Setenv("SYNOLOGY_SWEEP_FINDINGS_HELPER", "1")
	_, err := RunCLICommand(context.Background(), os.Args[0], []string{"-test.run=^TestFindingsHelperProcess$"})
	if err == nil {
		t.Fatal("the helper's exit 6 was not reported as an error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "need attention") || !strings.Contains(msg, `"sync_error":"login refused"`) {
		t.Fatalf("the error result lost the report rows or the summary: %s", msg)
	}
}
