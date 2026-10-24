// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): regression tests for the batch-1b Codex
// round-2 inputs, pinned by skills/cipp/handfixes.json under
// "tenant-list-message-is-not-a-fleet" and "fanout-save-integrity".

package cli

import "testing"

// TestFanoutNullBodyKeepsStoredUsers: Codex's input. A bare JSON null (and an
// empty body) is not the user list; u1 and u2 stay.
func TestFanoutNullBodyKeepsStoredUsers(t *testing.T) {
	for _, body := range []string{`null`, ` `} {
		if err, n := fanoutUsersTwice(t, body); err == nil || n != 2 {
			t.Fatalf("%q replaced the stored users: err=%v, %d left", body, err, n)
		}
	}
}

// TestFanoutNestedStatusFailureKeepsStoredUsers: Codex's input. A failure
// state nested under a status-like key beside a partial list refuses it; u2
// is not deleted.
func TestFanoutNestedStatusFailureKeepsStoredUsers(t *testing.T) {
	err, n := fanoutUsersTwice(t, `{"Results":[{"id":"u1"}],"status":{"code":"error"}}`)
	if err == nil || n != 2 {
		t.Fatalf("a nested status failure was accepted: err=%v, %d left", err, n)
	}
}

// TestFanoutDuplicateKeyKeepsStoredUsers: a duplicate key at any depth refuses
// the list; u1 and u2 stay.
func TestFanoutDuplicateKeyKeepsStoredUsers(t *testing.T) {
	for _, body := range []string{
		`{"Results":[{"id":"u1"},{"id":"u2"}],"Results":[{"id":"u1"}]}`,
		`[{"id":"u1"},{"id":"u2","id":"u3"}]`,
	} {
		if err, n := fanoutUsersTwice(t, body); err == nil || n != 2 {
			t.Fatalf("%s: a duplicate key was accepted: err=%v, %d left", body, err, n)
		}
	}
}
