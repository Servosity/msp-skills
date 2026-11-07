// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"net/http"
	"strings"
	"testing"
)

// Found by the guard simulation (seeds 11416, 12059, 20185): when a RESTORE
// task exists that may be an UNKNOWN row's but the watch cannot settle the row
// on it (another batch sent the same restore within the clock slack), the
// watch left the row UNKNOWN and 'restore submit --resume --retry-unknown'
// sent it again: the mailbox was restored twice. --retry-unknown now looks at
// the task list first, as a download resume does, and never resends a row a
// task may already be: such a row stays UNKNOWN for a person to check.
func TestRetryUnknownNeverResendsARowATaskMayBe(t *testing.T) {
	isolateAuthState(t)
	api := newFakeRestoreAPI(t, 0)
	// The send creates the task and its answer is lost.
	api.setStatus("a@contoso.com", http.StatusBadGateway)
	api.mu.Lock()
	api.acceptAnyway["a@contoso.com"] = true
	api.mu.Unlock()
	if _, _, err := runAuthCmd(t, "", submitArgs("--date", "2026-10-01", "--entities", "a@contoso.com")...); err == nil {
		t.Fatal("want the UNKNOWN exit")
	}
	api.setStatus("a@contoso.com", http.StatusOK)
	posts := api.entityPosts("a@contoso.com")
	_, stderr, err := runAuthCmd(t, "", submitArgs("--date", "2026-10-01", "--entities", "a@contoso.com", "--resume", "--retry-unknown")...)
	if got := api.entityPosts("a@contoso.com"); got != posts {
		t.Fatalf("--retry-unknown resent a restore a RESTORE task may already be (%d post(s))", got-posts)
	}
	if err == nil || ExitCode(err) != 5 {
		t.Fatalf("the held row is still UNKNOWN: want exit 5, got %v", err)
	}
	if !strings.Contains(err.Error()+stderr, "a RESTORE task that may be") {
		t.Fatalf("the run should say why it held the row:\n%v\n%s", err, stderr)
	}
	if strings.Contains(err.Error()+stderr, "--new-batch") && !strings.Contains(err.Error()+stderr, "on purpose") {
		t.Fatalf("must not steer to --new-batch:\n%v\n%s", err, stderr)
	}

	// With no such task, --retry-unknown sends it.
	isolateAuthState(t)
	api = newFakeRestoreAPI(t, 0)
	api.setStatus("a@contoso.com", http.StatusBadGateway)
	if _, _, err := runAuthCmd(t, "", submitArgs("--date", "2026-10-01", "--entities", "a@contoso.com")...); err == nil {
		t.Fatal("want the UNKNOWN exit")
	}
	api.setStatus("a@contoso.com", http.StatusOK)
	if _, stderr, err := runAuthCmd(t, "", submitArgs("--date", "2026-10-01", "--entities", "a@contoso.com", "--resume", "--retry-unknown")...); err != nil {
		t.Fatalf("a row with no task that could be it is sent again: %v\n%s", err, stderr)
	}
	if got := api.entityPosts("a@contoso.com"); got != 2 {
		t.Fatalf("posts = %d, want the original and the retry", got)
	}
}
