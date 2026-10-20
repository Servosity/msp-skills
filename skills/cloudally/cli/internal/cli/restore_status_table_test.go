// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"strings"
	"testing"
)

// Every RESTORE task status, run end to end through 'restore watch --once'
// against the stub: exit 0 only for the success allow-list, 8 while
// running, 5 for a failure, a partial or stopped restore, and any status
// the CLI does not recognise (named in the message, never read as success).
func TestRestoreWatchExitCodePerTaskStatus(t *testing.T) {
	isolateAuthState(t)
	api := newFakeRestoreAPI(t, 0)
	batchID := submitOneMailbox(t, "a@contoso.com")
	cases := []struct {
		status string
		exit   int
	}{
		{"COMPLETED", 0}, {"completed", 0}, {"COMPLETE", 0}, {"SUCCEEDED", 0}, {"SUCCEED", 0}, {"SUCCESS", 0}, {"DONE", 0}, {"FINISHED", 0},

		{"PENDING", ExitJobPending}, {"QUEUED", ExitJobPending}, {"SCHEDULED", ExitJobPending}, {"DISPATCHED", ExitJobPending},
		{"INPROCESS", ExitJobPending}, {"IN_PROCESS", ExitJobPending}, {"INPROGRESS", ExitJobPending}, {"IN_PROGRESS", ExitJobPending},
		{"RUNNING", ExitJobPending}, {"ACTIVE", ExitJobPending}, {"PAUSED", ExitJobPending},

		{"FAILED", 5}, {"PARTIALLY_SUCCEEDED", 5}, {"PARTIAL", 5}, {"COMPLETED_WITH_ERRORS", 5}, {"ABORTED", 5}, {"STOPPED", 5},
		{"CANCELLED", 5}, {"CANCELED", 5}, {"ERROR", 5},

		{"ARCHIVED", 5}, {"NOTACTIVE", 5}, {"READY_FOR_DOWNLOAD", 5}, {"SOMETHING_NEW", 5},
	}
	for _, tc := range cases {
		api.set(func(f *fakeRestoreAPI) { f.taskStatus = tc.status })
		_, _, err := runAuthCmd(t, "", "restore", "watch", "--batch", batchID, "--once", "--json")
		got := 0
		if err != nil {
			got = ExitCode(err)
		}
		if got != tc.exit {
			t.Errorf("task status %q: exit %d, want %d (%v)", tc.status, got, tc.exit, err)
			continue
		}
		outcome := restoreTaskOutcome(tc.status)
		if tc.exit == 5 && outcome == taskUnrecognized && !strings.Contains(err.Error(), "does not recognise as success") {
			t.Errorf("status %q: the message should flag it as unrecognised: %v", tc.status, err)
		}
		if tc.exit == 5 && outcome == taskUnrecognized && !strings.Contains(err.Error(), `"`+strings.ToUpper(tc.status)+`"`) {
			t.Errorf("status %q: the message should print the raw status: %v", tc.status, err)
		}
	}
}
