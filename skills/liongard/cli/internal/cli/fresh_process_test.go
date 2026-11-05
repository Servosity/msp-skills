// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"os"
	"os/exec"
	"regexp"
	"strings"
	"testing"
)

// freshProcessEnv names the one test a child test process runs in-process.
const freshProcessEnv = "LIONGARD_TEST_FRESH_PROCESS"

// inFreshProcess runs the calling test in a child test process and reports
// the child's verdict as its own. It returns true in the parent, which then
// returns at once, and false in the child, where the test body runs.
//
// A test that makes the credential kit hold a value (config.Load composing
// the access key pair holds the derived forms) must run here. The kit holds a
// value until the process exits, and while anything is held it checks every
// SQLite statement and file write in the process against the held set. Run
// in-process, one such test made every later store test in this package 10
// to 100 times slower (.review/seen/h4-held-set-contamination-seen-failing.log)
// and pushed the package past go test's 10-minute limit.
func inFreshProcess(t *testing.T) bool {
	t.Helper()
	if os.Getenv(freshProcessEnv) == t.Name() {
		return false
	}
	cmd := exec.Command(os.Args[0], "-test.run=^"+regexp.QuoteMeta(t.Name())+"$", "-test.count=1", "-test.v")
	cmd.Env = append(os.Environ(), freshProcessEnv+"="+t.Name())
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s in a fresh process: %v\n%s", t.Name(), err, out)
	}
	if !strings.Contains(string(out), "--- PASS: "+t.Name()+" ") {
		t.Fatalf("the fresh process did not run %s:\n%s", t.Name(), out)
	}
	return true
}
