// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"net/http"
	"testing"
)

// Found by the guard simulation (seed 10309): batch A's restore of a mailbox
// went UNKNOWN and never happened; batch B (--new-batch) sent it again, and
// that send created a RESTORE task whose answer was lost. 'restore watch' on
// A took B's task as A's (same mailbox, created after A's send) and settled
// A's row; B's row then found its task pinned to A, stayed UNKNOWN, and the
// --retry-unknown its watch led to restored the mailbox a second time.
//
// A task created once the identity was sent again by another batch is not
// the earlier row's: the earlier row only takes a task created clearly before
// that next send (the clock slack included), and otherwise stays UNKNOWN.
// The task list still holds the task, so --retry-unknown sends nothing.
func TestRestoreWatchNeverTakesALaterBatchsTask(t *testing.T) {
	isolateAuthState(t)
	api := newFakeRestoreAPI(t, 0)
	batchOf := func(out string) string {
		var v struct {
			BatchID string `json:"batch_id"`
		}
		if err := json.Unmarshal([]byte(out), &v); err != nil || v.BatchID == "" {
			t.Fatalf("no batch id in submit output: %v\n%s", err, out)
		}
		return v.BatchID
	}
	// A: the send fails with a 502 and creates nothing.
	api.setStatus("a@contoso.com", http.StatusBadGateway)
	out, _, err := runAuthCmd(t, "", submitArgs("--date", "2026-10-01", "--entities", "a@contoso.com")...)
	if err == nil {
		t.Fatal("A: want the UNKNOWN exit")
	}
	a := batchOf(out)
	// B: --new-batch sends it again; this one creates the task, and its
	// answer is lost (502).
	api.mu.Lock()
	api.acceptAnyway["a@contoso.com"] = true
	api.mu.Unlock()
	out, _, err = runAuthCmd(t, "", submitArgs("--date", "2026-10-01", "--entities", "a@contoso.com", "--new-batch")...)
	if err == nil {
		t.Fatal("B: want the UNKNOWN exit")
	}
	b := batchOf(out)

	wa, _ := watchJSON(t, a, "--once")
	if len(wa.Rows) != 1 || wa.Rows[0].submitOutcome() != jobUnknown || wa.Rows[0].TaskID != "" {
		t.Fatalf("A's watch took B's task: %+v", wa.Rows)
	}
	// Nor does B take it: A's unpinned send came inside the slack before
	// B's, so the one task may be either's and B's row stays UNKNOWN
	// (Codex round 1, TestRestoreWatchNeverTakesAnEarlierBatchsTask).
	wb, _ := watchJSON(t, b, "--once")
	if len(wb.Rows) != 1 || wb.Rows[0].submitOutcome() != jobUnknown || wb.Rows[0].TaskID != "" {
		t.Fatalf("B's watch settled a row on a task that may be A's: %+v", wb.Rows)
	}
	// So --retry-unknown on B sends nothing, and the mailbox is restored once.
	posts := api.entityPosts("a@contoso.com")
	_, _, _ = runAuthCmd(t, "", submitArgs("--date", "2026-10-01", "--entities", "a@contoso.com", "--resume", "--retry-unknown")...)
	if got := api.entityPosts("a@contoso.com"); got != posts {
		t.Fatalf("--retry-unknown resent a restore whose task exists (%d posts)", got-posts)
	}
}

// Codex round 1, the reverse: batch A's send was accepted and created a task
// that A never watched; batch B (--new-batch) then sent the same restore and
// got a 502 that created nothing. Watching B first took A's task (the only
// one, created inside B's slack) and settled B's UNKNOWN row as SUBMITTED. A
// task created where another batch's unpinned send could have made it is
// not this row's: the evidence cannot decide, so B's row stays UNKNOWN.
func TestRestoreWatchNeverTakesAnEarlierBatchsTask(t *testing.T) {
	isolateAuthState(t)
	api := newFakeRestoreAPI(t, 0)
	batchOf := func(out string) string {
		var v struct {
			BatchID string `json:"batch_id"`
		}
		if err := json.Unmarshal([]byte(out), &v); err != nil || v.BatchID == "" {
			t.Fatalf("no batch id in submit output: %v\n%s", err, out)
		}
		return v.BatchID
	}
	out, _, err := runAuthCmd(t, "", submitArgs("--date", "2026-10-01", "--entities", "a@contoso.com")...)
	if err != nil {
		t.Fatalf("A: %v", err)
	}
	a := batchOf(out)
	api.setStatus("a@contoso.com", http.StatusBadGateway)
	out, _, err = runAuthCmd(t, "", submitArgs("--date", "2026-10-01", "--entities", "a@contoso.com", "--new-batch")...)
	if err == nil {
		t.Fatal("B: want the UNKNOWN exit")
	}
	b := batchOf(out)
	if b == a {
		t.Fatal("B must be a new batch")
	}

	wb, _ := watchJSON(t, b, "--once")
	if len(wb.Rows) != 1 || wb.Rows[0].submitOutcome() != jobUnknown || wb.Rows[0].TaskID != "" {
		t.Fatalf("B's watch took A's task: %+v", wb.Rows)
	}
}
