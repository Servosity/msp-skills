// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package store

import (
	"os"
	"os/exec"
	"testing"
)

// runInChild runs the calling top-level test alone in a child process and
// reports true in the parent, which should return. A credential kit hold lasts
// until the process exits, so a test that holds a value runs where it cannot
// reach any other test's files and statements.
func runInChild(t *testing.T) bool {
	t.Helper()
	const marker = "HS_TEST_HOLD_CHILD"
	if os.Getenv(marker) == t.Name() {
		return false
	}
	cmd := exec.Command(os.Args[0], "-test.run=^"+t.Name()+"$", "-test.count=1")
	cmd.Env = append(os.Environ(), marker+"="+t.Name())
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("child: %v\n%s", err, out)
	}
	return true
}
