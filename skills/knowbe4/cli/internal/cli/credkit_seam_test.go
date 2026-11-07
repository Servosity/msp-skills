// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"net/http"
	"os"
	"os/exec"
	"testing"

	"knowbe4-pp-cli/internal/cliutil"
	"knowbe4-pp-cli/internal/credkit"
)

// In-process tests run without the credential kit's outbound guard and holds
// (see cliutil.OutboundGuard, cliutil.HoldCredential): none runs credkit.Start,
// so the guard would refuse every credentialed request to the loopback servers
// they stand up, and a held token would be checked against every later test.
// The built binaries' tests (internal/knowbe4blackbox) and child-process tests
// (runInChild) prove both.
func init() {
	cliutil.OutboundGuard = func(rt http.RoundTripper) http.RoundTripper {
		if rt == nil {
			return http.DefaultTransport // as credkit.Transport reads nil
		}
		return rt
	}
	cliutil.HoldCredential = func(credkit.Source, ...string) {}
}

// runInChild runs the calling top-level test alone in a child process and
// reports true in the parent, which should return. A credential kit hold lasts
// until the process exits, so a test that holds a value runs where it cannot
// reach any other test's files and statements.
func runInChild(t *testing.T) bool {
	t.Helper()
	const marker = "KNOWBE4_TEST_HOLD_CHILD"
	if os.Getenv(marker) == t.Name() {
		return false
	}
	cmd := exec.Command(os.Args[0], "-test.run=^"+t.Name()+"$", "-test.count=1") // #nosec G204 -- this test binary
	cmd.Env = append(os.Environ(), marker+"="+t.Name())
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("child: %v\n%s", err, out)
	}
	return true
}
