// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"net/http"

	"pagerduty-pp-cli/internal/client"

	"os"

	"os/exec"

	"testing"
)

// No in-process test runs credkit.Start, so the outbound guard would refuse
// every credentialed request and then fail closed for the whole test process.
// The built binaries' tests (internal/credwire) prove it.
func init() {
	client.OutboundGuard = func(rt http.RoundTripper) http.RoundTripper { return rt }
}

// inFreshProcess reruns the calling test alone in a child process and reports
// whether it did. A held value lasts for the process, and with one held the kit
// checks every later statement and write against it, so a test that holds a
// value must not share a process with the rest of the package.
func inFreshProcess(t *testing.T) bool {
	t.Helper()
	const marker = "PAGERDUTY_TEST_FRESH_PROCESS"
	if os.Getenv(marker) == t.Name() {
		return false
	}
	cmd := exec.Command(os.Args[0], "-test.run=^"+t.Name()+"$", "-test.count=1") // #nosec G204 -- this test binary
	cmd.Env = append(os.Environ(), marker+"="+t.Name())
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	return true
}
