//go:build windows

// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cliutil

import "testing"

// checkFixedDispatchLockIdentity: the lock is a Global\ kernel object named by
// the account SID from the process token, never a path.
func checkFixedDispatchLockIdentity(t *testing.T, got string) {
	t.Helper()
	sid, err := currentUserSID()
	if err != nil {
		t.Fatal(err)
	}
	if want := `Global\comet-backup-cli-` + testDispatchLock + `-` + sid; got != want {
		t.Fatalf("lock = %q, want %q", got, want)
	}
}
