// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// The CLI binary (and the MCP server, which runs this package) embeds the
// IANA zone database, so scheduled-policy zones resolve on Windows without Go
// or ZONEINFO. A system lookup cannot prove this on a machine that has
// zoneinfo, so the test asks the toolchain whether time/tzdata is linked in,
// for the host and for windows/amd64.
func TestBinaryEmbedsTZData(t *testing.T) {
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go toolchain not on PATH")
	}
	for _, goos := range []string{"", "windows"} {
		for _, pkg := range []string{"../../cmd/dnsfilter-cli", "../../cmd/dnsfilter-mcp"} {
			cmd := exec.Command(goBin, "list", "-deps", pkg)
			cmd.Env = os.Environ()
			if goos != "" {
				cmd.Env = append(cmd.Env, "GOOS="+goos, "GOARCH=amd64")
			}
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("go list -deps %s (GOOS=%s): %v\n%s", pkg, goos, err, out)
			}
			found := false
			for _, line := range strings.Split(string(out), "\n") {
				if strings.TrimSpace(line) == "time/tzdata" {
					found = true
				}
			}
			if !found {
				t.Fatalf("%s (GOOS=%q) does not link time/tzdata; IANA zones will not resolve on Windows without Go", pkg, goos)
			}
		}
	}
}
