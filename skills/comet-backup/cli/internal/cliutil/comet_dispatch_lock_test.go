// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// comet-backup handfix (rerun-dispatch-lock): the dispatch lock is found from
// the OS identity alone and is exclusive within and across goroutines.

package cliutil

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"comet-backup-pp-cli/internal/cliutil/testenv"
)

// dispatchLockEnvVars is every variable a path lookup on any platform reads.
var dispatchLockEnvVars = []string{
	"HOME", "USERPROFILE", "HOMEDRIVE", "HOMEPATH",
	"XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME", "XDG_CACHE_HOME", "XDG_RUNTIME_DIR",
	"TMPDIR", "TMP", "TEMP", "LOCALAPPDATA", "APPDATA", "SystemDrive", "SystemRoot",
	"COMET_BACKUP_HOME", "COMET_BACKUP_CONFIG", "COMET_BACKUP_CONFIG_DIR",
	"COMET_BACKUP_DATA_DIR", "COMET_BACKUP_STATE_DIR", "COMET_BACKUP_CACHE_DIR",
}

const testDispatchLock = "cliutil-test-dispatch"

func TestDispatchLockIdentityIgnoresEnvironment(t *testing.T) {
	restore, err := SetHomeOverride("")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(restore)
	testenv.Isolate(t, ConfigDir, DataDir, StateDir, CacheDir)
	want, err := dispatchLockIdentity(testDispatchLock)
	if err != nil {
		t.Fatal(err)
	}
	checkFixedDispatchLockIdentity(t, want)
	for _, name := range dispatchLockEnvVars {
		t.Run(name, func(t *testing.T) {
			t.Setenv(name, t.TempDir())
			if got, err := dispatchLockIdentity(testDispatchLock); err != nil || got != want {
				t.Fatalf("with %s moved the lock is %q (err %v), want %q", name, got, err, want)
			}
		})
	}
	t.Run("--home", func(t *testing.T) {
		restore, err := SetHomeOverride(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		defer restore()
		if got, err := dispatchLockIdentity(testDispatchLock); err != nil || got != want {
			t.Fatalf("with --home the lock is %q (err %v), want %q", got, err, want)
		}
	})
}

func TestTryWithDispatchLockIsExclusive(t *testing.T) {
	testenv.Isolate(t)
	holding, release, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() {
		_, err := TryWithDispatchLock(testDispatchLock, func() error {
			close(holding)
			<-release
			return nil
		})
		done <- err
	}()
	select {
	case <-holding:
	case err := <-done:
		t.Fatalf("the first holder could not take the lock: %v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("the first holder did not take the lock")
	}
	ran, err := TryWithDispatchLock(testDispatchLock, func() error {
		t.Error("a second holder ran while the first held the lock")
		return nil
	})
	if ran || err != nil {
		t.Fatalf("second try = (%v, %v), want (false, nil)", ran, err)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	ran, err = TryWithDispatchLock(testDispatchLock, func() error { return nil })
	if !ran || err != nil {
		t.Fatalf("after release = (%v, %v), want (true, nil)", ran, err)
	}
}

func TestTryWithDispatchLockRejectsBadNames(t *testing.T) {
	for _, name := range []string{"", "../escape", "a/b", `a\b`, "Upper", "-lead", strings.Repeat("a", 65)} {
		ran, err := TryWithDispatchLock(name, func() error {
			t.Errorf("%q: fn ran", name)
			return nil
		})
		if ran || err == nil {
			t.Errorf("%q = (%v, %v), want a refusal", name, ran, err)
		}
	}
}

func TestDispatchIntentLifecycle(t *testing.T) {
	key := "test-intent-" + t.Name() + "-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	t.Cleanup(func() { ClearDispatchIntent(key) })
	if _, found, err := DispatchIntentAge(key); err != nil || found {
		t.Fatalf("before: found=%v err=%v", found, err)
	}
	if err := RecordDispatchIntent(key); err != nil {
		t.Fatal(err)
	}
	if age, found, err := DispatchIntentAge(key); err != nil || !found || age < -time.Minute || age > time.Minute {
		t.Fatalf("after record: age=%v found=%v err=%v", age, found, err)
	}
	ClearDispatchIntent(key)
	if _, found, err := DispatchIntentAge(key); err != nil || found {
		t.Fatalf("after clear: found=%v err=%v", found, err)
	}
}
