// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"os"
	"os/exec"
	"testing"
)

// inFreshProcess reruns the calling test alone in a child process and reports
// whether it did. A held value lasts for the process, and with one held the kit
// withholds some ordinary statements on budget, so a
// test that holds a value must not share a process with the rest of the package.
func inFreshProcess(t *testing.T) bool {
	t.Helper()
	const marker = "AWS_BILLING_TEST_FRESH_PROCESS"
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
