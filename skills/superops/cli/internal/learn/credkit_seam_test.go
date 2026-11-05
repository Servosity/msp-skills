// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package learn_test

import (
	"os"
	"os/exec"
	"testing"
)

const heldChildEnv = "SUPEROPS_TEST_HELD_CHILD"

// inHeldChild reports whether this test binary is the copy heldChild started.
func inHeldChild() bool { return os.Getenv(heldChildEnv) != "" }

// heldChild runs the calling test alone in a fresh copy of the test binary and
// reports whether the caller is that copy. The kit's held set is process-wide
// and has no reset, so a test that holds a value runs where no other test's
// statements or files are checked against it.
func heldChild(t *testing.T) bool {
	t.Helper()
	if inHeldChild() {
		return true
	}
	cmd := exec.Command(os.Args[0], "-test.run=^"+t.Name()+"$", "-test.count=1")
	cmd.Env = append(os.Environ(), heldChildEnv+"=1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s in its own process: %v\n%s", t.Name(), err, out)
	}
	return false
}
