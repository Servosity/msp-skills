// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package main

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// microsoft-graph handfix (mcp-startup-error-delivered): stderr passes the
// output funnel's pipe, so a startup error written just before os.Exit was
// lost and the server exited 1 in silence. The real main runs in a child
// process (the funnel is installed by init, as in the shipped binary).
func TestHTTPWithoutTokenNamesTheVariable(t *testing.T) {
	if os.Getenv("MG_MCP_TEST_RUN_MAIN") == "1" {
		os.Args = []string{"microsoft-graph-mcp", "--transport", "http"}
		main()
		return
	}
	home := t.TempDir()
	for i := 0; i < 5; i++ {
		cmd := exec.Command(os.Args[0], "-test.run=^TestHTTPWithoutTokenNamesTheVariable$")
		cmd.Env = []string{"MG_MCP_TEST_RUN_MAIN=1", "HOME=" + home, "USERPROFILE=" + home, "MICROSOFT_GRAPH_HOME=" + home, "PATH=" + os.Getenv("PATH"), "SYSTEMROOT=" + os.Getenv("SYSTEMROOT")}
		out, err := cmd.CombinedOutput()
		if ee, ok := err.(*exec.ExitError); !ok || ee.ExitCode() != 1 {
			t.Fatalf("want exit 1, got %v: %s", err, out)
		}
		if !strings.Contains(string(out), httpTokenEnvVar) {
			t.Fatalf("run %d: the startup error was not delivered: %q", i, out)
		}
	}
}
