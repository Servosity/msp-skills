// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

const holdingChildEnv = "KBMS_CLI_HOLDING_CHILD"

// holdingTests hold a credential. The kit has no release, and once a process
// holds a value every statement on every store handle pays the kit's full
// scan: store-opening tests went from milliseconds to 10-20 s each in the Linux
// row, and the package ran past go test's ten-minute limit. These run together
// in one child process, so the rest of the package holds nothing.
var holdingTests = []string{
	"TestCredentialEndpointResultIsHeldUnderAnyName",
	"TestDryRunChangesNoLocalFile",
	"TestExportWalksEveryKbmsPageAndStops",
	"TestMintNeverLeavesTheAPIOrigin",
	"TestMintedTokenIsHeldAndSent",
	"TestOriginPinKeepsTheTokenOnTheAPIHost",
	"TestStoreOfAnotherHostIsNeverAdopted",
	"TestStoredCredentialNeverStandsInForTheConfiguredOne",
	"TestStoredTokenNeverReachesAnotherHost",
	"TestSyncPartitionHoldsOnlyItsOwnPath",
	"TestSyncReadsEveryPage",
}

// inHoldingChild is the first line of every holding test: outside the child
// process it skips, because TestHoldingTestsInTheirOwnProcess runs it there.
func inHoldingChild(t *testing.T) {
	t.Helper()
	if os.Getenv(holdingChildEnv) != "1" {
		t.Skip("runs in the child process of TestHoldingTestsInTheirOwnProcess")
	}
}

// A failure in the child, or a holding test the child did not run, fails here.
func TestHoldingTestsInTheirOwnProcess(t *testing.T) {
	if os.Getenv(holdingChildEnv) == "1" {
		t.Skip("this is the child process")
	}
	cmd := exec.Command(os.Args[0], "-test.run=^("+strings.Join(holdingTests, "|")+")$", "-test.count=1", "-test.v")
	cmd.Env = append(os.Environ(), holdingChildEnv+"=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("the holding tests failed in their own process (err=%v):\n%s", err, out)
	}
	for _, name := range holdingTests {
		if !strings.Contains(string(out), "--- PASS: "+name+" ") && !strings.Contains(string(out), "--- SKIP: "+name+" ") {
			t.Errorf("%s did not run in the child process:\n%s", name, out)
		}
	}
}
