// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cliutil

// comet-backup handfix (rerun-dispatch-lock): one dispatch lock per OS
// account, found from the OS identity alone. Neither its location nor its
// name derives from an environment variable, a flag or a config path (HOME,
// USERPROFILE, XDG_*, TMPDIR/TMP/TEMP, LOCALAPPDATA/APPDATA,
// COMET_BACKUP_HOME, --home, --config): any of those differs between two
// invocations by the same account (another terminal, an agent, a scheduler),
// and a lock that follows it is two locks, so both runs plan the same failed
// backup and both dispatch it (Codex: HOME=/tmp/comet-a vs HOME=/tmp/comet-b).
//
// The name is not keyed by the Comet Server either. One server is reached as
// backup.example.com, backup.example.com:443, an alias, a CNAME or its IP, and
// by more than one admin account; any key built from that text splits it
// again (publish Codex r1). Runs against different servers wait for each
// other; that costs time, never a duplicate dispatch.
//
// Unix: an flock on <name>.lock inside /tmp/comet-backup-locks-<euid>, a
// fixed absolute path (never os.TempDir, which reads TMPDIR). The directory is
// created 0700 and must prove itself private before anything is read or
// dispatched: a real directory (not a symlink), owned by this account, not
// group or world writable, no darwin ACL entry granting another principal
// write. Anything else fails closed. Windows: a named kernel mutex keyed on
// the account SID (comet_dispatch_lock_windows.go says why).

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"
)

// ErrDispatchLockUnsafe marks a refusal: the lock could not be proven private
// to this account, so the caller must not read a plan or dispatch anything.
var ErrDispatchLockUnsafe = errors.New("refusing to dispatch: the dispatch lock is not private to this account")

var dispatchLockName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)

// dispatchLocks pairs each OS lock with a process mutex, so two goroutines of
// one process (the MCP server) serialize as two processes do.
var dispatchLocks sync.Map // name -> *sync.Mutex

// TryWithDispatchLock runs fn while holding this OS account's dispatch lock
// called name and reports true, or reports false without running fn while
// another holder, in this process or any other, has it. The OS releases the
// lock when a holder dies. A non-nil error with false means fn did not run:
// the lock was refused (ErrDispatchLockUnsafe) or could not be taken.
func TryWithDispatchLock(name string, fn func() error) (bool, error) {
	if !dispatchLockName.MatchString(name) {
		return false, fmt.Errorf("invalid dispatch lock name %q", name)
	}
	v, _ := dispatchLocks.LoadOrStore(name, &sync.Mutex{})
	mu, _ := v.(*sync.Mutex)
	if !mu.TryLock() {
		return false, nil
	}
	defer mu.Unlock()
	return tryOSDispatchLock(name, fn)
}

func dispatchLockRefusal(format string, args ...any) error {
	return fmt.Errorf("%w: %s; nothing was read or dispatched", ErrDispatchLockUnsafe, fmt.Sprintf(format, args...))
}

// comet-backup handfix (rerun-dispatch-intent): a dispatch intent is an empty file
// written before a dispatch is sent and removed once the server has answered. One
// that outlives its run means the outcome is unknown: the backup may have started,
// and the job may not show yet. It lives beside the lock, found the same way, and
// holds only a hash of its key.
func dispatchIntentPath(key string) (string, error) {
	dir, err := dispatchIntentDir()
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(key))
	return filepath.Join(dir, "intent-"+hex.EncodeToString(sum[:])), nil
}

// RecordDispatchIntent notes that a dispatch for key is about to be sent. On an
// error the caller must not send it.
func RecordDispatchIntent(key string) error {
	path, err := dispatchIntentPath(key)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Clean(path), os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("recording the dispatch: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("recording the dispatch: %w", err)
	}
	now := time.Now()
	return os.Chtimes(path, now, now)
}

// DispatchIntentAge reports how long ago a dispatch for key was recorded and not
// answered. On an error the caller must not send it.
func DispatchIntentAge(key string) (time.Duration, bool, error) {
	path, err := dispatchIntentPath(key)
	if err != nil {
		return 0, false, err
	}
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("reading the dispatch record: %w", err)
	}
	return time.Since(info.ModTime()), true, nil
}

// ClearDispatchIntent removes the record for key.
func ClearDispatchIntent(key string) {
	if path, err := dispatchIntentPath(key); err == nil {
		_ = os.Remove(path)
	}
}
