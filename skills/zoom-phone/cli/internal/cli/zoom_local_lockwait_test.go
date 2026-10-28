// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Codex round-11: waiting for the store lock honors the caller's timeout and
// never leaves a late acquirer behind.

package cli

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLocalLockWaitHonorsTimeout(t *testing.T) {
	localTestEnv(t)
	localNumbersOnlyServer(t)
	path := filepath.Join(t.TempDir(), "data.db")
	release := localHoldStoreLock(t, path)
	released := false
	t.Cleanup(func() {
		if !released {
			release()
		}
	})

	cases := []struct {
		name string
		args []string
		want string
	}{
		{"workflow archive --timeout 300ms", []string{"workflow", "archive", "--db", path, "--timeout", "300ms", "--json"}, "timed out after 300ms"},
		{"sync --timeout 300ms", []string{"sync", "--full", "--resources", "numbers", "--db", path, "--timeout", "300ms", "--json"}, "gave up waiting for another sync"},
	}
	for _, tc := range cases {
		done := make(chan error, 1)
		go func() {
			cmd := RootCmd()
			var out, errb zoomLockedBuffer
			cmd.SetOut(&out)
			cmd.SetErr(&errb)
			cmd.SetArgs(tc.args)
			done <- cmd.Execute()
		}()
		select {
		case err := <-done:
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("%s: err=%v, want %q", tc.name, err, tc.want)
			}
		case <-time.After(3 * time.Second):
			release()
			released = true
			<-done
			t.Fatalf("%s blocked behind the held lock instead of honoring its timeout", tc.name)
		}
	}

	// Nothing that gave up may still be waiting to take the lock.
	release()
	released = true
	time.Sleep(300 * time.Millisecond)
	r, ok, err := localTryHoldLock(localSyncLockPath(path))
	if err != nil || !ok {
		t.Fatalf("lock not acquirable after the timed-out waiters returned (ok=%v err=%v): an orphaned waiter took it", ok, err)
	}
	r()
}
