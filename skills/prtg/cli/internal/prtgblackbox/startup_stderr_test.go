// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package prtgblackbox

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// prtg handfix (mcp-exit-flushes-stderr): every MCP startup refusal reaches
// the operator. The stderr filter is asynchronous, so an exit that does not
// wait for it printed nothing at all.
func TestMCPStartupErrorsReachStderr(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
		code int
	}{
		{[]string{"--transport", "http"}, "PRTG_MCP_HTTP_TOKEN", 1},
		{[]string{"--transport", "bogus"}, "unknown --transport", 2},
	} {
		cmd := exec.Command(bbMCP, tc.args...)
		cmd.Env = []string{"HOME=" + t.TempDir(), "PATH=/usr/bin:/bin", "PRTG_HOME=" + t.TempDir(), "SYSTEMROOT=" + os.Getenv("SYSTEMROOT")}
		out, err := cmd.CombinedOutput()
		ee, ok := err.(*exec.ExitError)
		if !ok || ee.ExitCode() != tc.code || !strings.Contains(string(out), tc.want) {
			t.Errorf("%v: want exit %d naming %q, got %v:\n%s", tc.args, tc.code, tc.want, err, out)
		}
	}
}
