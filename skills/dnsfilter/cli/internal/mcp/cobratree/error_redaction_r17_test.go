// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/dnsfilter/handfixes.json
// under the "mcp-read-only-hinted-tools-filter-credentials" entry.

package cobratree

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"dnsfilter-pp-cli/internal/cliutil"
	"dnsfilter-pp-cli/internal/mcp/redact"
)

// r17ChildEnv turns this test binary into a stand-in companion CLI: the tests
// below run RunCLICommand on os.Args[0], and init answers in the given mode
// instead of running tests (portable: no shell script, no chmod).
const r17ChildEnv = "DNSFILTER_COBRATREE_TEST_R17_CHILD"

const (
	r17ChildTailSecret = "SENTINEL-CHILD-TAIL-KEY"
	r17ChildHintSecret = "SENTINEL-CHILD-HINT-PIN"
)

func init() {
	switch os.Getenv(r17ChildEnv) {
	case "":
		return
	case "tail-cut":
		// One JSON object over 25 lines, its secret's key and value on
		// separate lines, so the 20-line tail failedStdoutDetail keeps
		// starts at the bare value.
		fmt.Println("{")
		for i := 0; i < 3; i++ {
			fmt.Printf("\"field_%d\": %d,\n", i, i)
		}
		fmt.Println(`"secret_key":`)
		fmt.Printf("%q,\n", r17ChildTailSecret)
		for i := 0; i < 17; i++ {
			fmt.Printf("\"filler_%02d\": %d,\n", i, i)
		}
		fmt.Println(`"name": "Main Office"`)
		fmt.Println("}")
		fmt.Fprintln(os.Stderr, "Error: why-blocked failed")
		os.Exit(1)
	case "hint":
		fmt.Fprintln(os.Stderr, "hint: Uninstall PIN: "+r17ChildHintSecret)
		fmt.Println(`{"ok":true}`)
		os.Exit(0)
	}
	os.Exit(0)
}

// TestRunCLICommandRedactsChildOutputForReadOnlyHintedCalls: off the
// read-only switch, a call whose context the MCP result funnel marked
// (redact.WithForced: the tool is advertised readOnlyHint=true) redacts the
// captured stdout and stderr whole before failedStdoutDetail tails it and
// before stderrHintLines selects from it. An unmarked call (a
// destructive-hinted tool such as tail) is unchanged, which also proves the
// fixtures reach the leak.
func TestRunCLICommandRedactsChildOutputForReadOnlyHintedCalls(t *testing.T) {
	t.Setenv(cliutil.MCPReadOnlyEnvVar, "")

	t.Setenv(r17ChildEnv, "tail-cut")
	_, err := RunCLICommand(context.Background(), os.Args[0], nil)
	if err == nil || !strings.Contains(err.Error(), r17ChildTailSecret) {
		t.Fatalf("unmarked call: the fixture does not put the bare value in the error tail: %v", err)
	}
	_, err = RunCLICommand(redact.WithForced(context.Background()), os.Args[0], nil)
	if err == nil {
		t.Fatal("tail-cut helper succeeded")
	}
	if strings.Contains(err.Error(), r17ChildTailSecret) {
		t.Fatalf("read-only-hinted call: the error tail kept a value cut from its key: %v", err)
	}
	if !strings.Contains(err.Error(), "why-blocked failed") || !strings.Contains(err.Error(), "Main Office") {
		t.Fatalf("read-only-hinted call lost the non-secret error detail: %v", err)
	}

	t.Setenv(r17ChildEnv, "hint")
	out, err := RunCLICommand(context.Background(), os.Args[0], nil)
	if err != nil || !strings.Contains(strings.Join(out.StderrHints, "\n"), r17ChildHintSecret) {
		t.Fatalf("unmarked call: the fixture's hint line is not selected (%v): %q", err, out.StderrHints)
	}
	out, err = RunCLICommand(redact.WithForced(context.Background()), os.Args[0], nil)
	if err != nil {
		t.Fatalf("hint helper failed: %v", err)
	}
	if hints := strings.Join(out.StderrHints, "\n"); strings.Contains(hints, r17ChildHintSecret) {
		t.Fatalf("read-only-hinted call: a stderr hint carried a credential: %q", hints)
	}
	if !strings.Contains(out.Stdout, `{"ok":true}`) {
		t.Fatalf("stdout changed: %q", out.Stdout)
	}
}
