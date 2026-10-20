// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"cloudally-pp-cli/internal/cliutil"
)

// batchKeyPattern is the shape of a batch key: a UTC timestamp, then short
// lower-case segments (the account prefix and a random suffix). It allows
// no separator and no dot, so a key can never name a path outside the
// batch folder. Keys made before the random suffix existed still match.
var batchKeyPattern = regexp.MustCompile(`^[0-9]{8}-[0-9]{6}(-[a-z0-9]{1,16}){0,3}$`)

// validBatchKey refuses anything that is not a batch key.
func validBatchKey(key string) error {
	if !batchKeyPattern.MatchString(key) {
		return fmt.Errorf("%q is not a batch key (one looks like 20261015-201500-5f1c-1a2b3c4d)", tenantText(key))
	}
	return nil
}

// newBatchKey makes a key that two runs started in the same second for the
// same account still cannot share.
func newBatchKey(now time.Time, account string) string {
	var r [4]byte
	_, _ = rand.Read(r[:])
	key := now.UTC().Format("20060102-150405")
	if short := shortKey(account); short != "" {
		key += "-" + short
	}
	return key + "-" + hex.EncodeToString(r[:])
}

// shortKey makes a stable short, path-safe prefix from an account id.
func shortKey(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			if b.Len() == 4 {
				break
			}
		}
	}
	return b.String()
}

// batchDir returns the folder that holds batch ledgers.
func batchDir() (string, error) {
	dir, err := cliutil.StateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, downloadBatchDirName), nil
}

// batchPath returns where a batch ledger lives. The key is validated, so
// the result is always a file directly inside the batch folder.
func batchPath(key string) (string, error) {
	if err := validBatchKey(key); err != nil {
		return "", err
	}
	dir, err := batchDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, key+".json"), nil
}

// saveBatch writes the ledger through a temporary file and a rename, so a
// run killed mid-write leaves the previous ledger rather than half of one.
// It is called once per row before and after each create, so that matters.
func saveBatch(b *downloadBatch) error {
	path, err := batchPath(b.BatchKey)
	if err != nil {
		return err
	}
	b.UpdatedAt = isoUTC(time.Now())
	data, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return err
	}
	return cliutil.AtomicWritePrivateFile(path, append(data, '\n'), batchFilePerm, 0o700)
}

// loadBatch reads one ledger. The read is confined to the batch folder (a
// link pointing out of it is refused), and a ledger whose own batch_key
// differs from the name it was asked for is refused, so a doctored file can
// never redirect later saves somewhere else.
func loadBatch(key string) (*downloadBatch, error) {
	if err := validBatchKey(key); err != nil {
		return nil, err
	}
	dir, err := batchDir()
	if err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, fmt.Errorf("no batch ledgers yet (%w)", err)
	}
	defer root.Close()
	name := key + ".json"
	fi, err := root.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !fi.Mode().IsRegular() {
		return nil, fmt.Errorf("batch %s is not a plain file", key)
	}
	data, err := root.ReadFile(name)
	if err != nil {
		return nil, err
	}
	var b downloadBatch
	if err := json.Unmarshal(data, &b); err != nil {
		return nil, fmt.Errorf("reading batch %s: %w", key, err)
	}
	if b.BatchKey != key {
		return nil, fmt.Errorf("the ledger for batch %s names itself %q; refusing it", key, tenantText(b.BatchKey))
	}
	return &b, nil
}

// errBatchInUse is a batch another run is working on right now.
var errBatchInUse = errors.New("another download submit run is working on this batch")

// withBatchLock runs fn while holding the batch's lock file (flock on
// macOS and Linux, LockFileEx on Windows). A second run, in this process
// or another, is refused rather than made to wait, so two runs can never
// send or save the same batch at once.
func withBatchLock(key string, fn func() error) error {
	path, err := batchPath(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	ran, err := cliutil.TryWithFileLock(path, fn)
	if err != nil {
		return err
	}
	if !ran {
		return usageErr(fmt.Errorf("batch %s: %w; nothing was sent. Wait for it to finish, then run the same command again", key, errBatchInUse))
	}
	return nil
}

// rerunGuardName is the lock file that makes the re-run check and the
// claim of a new batch one step across processes.
const rerunGuardName = "submit-guard"

// rerunGuardPause is a test hook: how long a claim waits between its check
// and its save, to widen the window a missing lock would leave open.
var rerunGuardPause time.Duration

// rerunIdentity is what makes two submits "the same command": scope,
// account, task, the date (or --latest), the entity and sub-source filters
// and the format. The timezone and --min-size are left out on purpose: a
// run that differs only in those still selects mostly the same archives,
// so it is refused too rather than quietly creating them twice.
func rerunIdentity(scope, account, task, onDate, format string, entities, subSources []string) string {
	return strings.Join([]string{
		"v1", scope, strings.ToLower(strings.TrimSpace(account)), strings.TrimSpace(task),
		strings.ToLower(onDate), strings.ToLower(format),
		strings.Join(entities, ","), strings.Join(subSources, ","),
	}, "|")
}

// ownedRows counts the rows of a batch that exist, may exist, or are about
// to be sent: everything except a create the portal refused or never saw.
func ownedRows(b *downloadBatch) int {
	n := 0
	for _, r := range b.Rows {
		if r.Status != dlFailed {
			n++
		}
	}
	return n
}

// findRerunBatch returns the newest earlier batch with this identity that
// still owns rows.
func findRerunBatch(identity string) (*downloadBatch, int, error) {
	dir, err := batchDir()
	if err != nil {
		return nil, 0, err
	}
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, 0, nil
	}
	if err != nil {
		return nil, 0, err
	}
	var best *downloadBatch
	owned := 0
	for _, e := range entries {
		key, ok := strings.CutSuffix(e.Name(), ".json")
		if !ok || validBatchKey(key) != nil {
			continue
		}
		b, err := loadBatch(key)
		if err != nil || b.RerunKey != identity {
			continue
		}
		if n := ownedRows(b); n > 0 && (best == nil || b.CreatedAt > best.CreatedAt) {
			best, owned = b, n
		}
	}
	return best, owned, nil
}

// claimNewBatch records a new batch, unless an earlier batch for the same
// command already owns rows and newBatch was not given. The check and the
// save run under one lock across processes, so two identical commands
// started together cannot both pass the check.
func claimNewBatch(b *downloadBatch, newBatch bool) error {
	dir, err := batchDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	return cliutil.WithFileLock(filepath.Join(dir, rerunGuardName), func() error {
		if !newBatch {
			prior, n, err := findRerunBatch(b.RerunKey)
			if err != nil {
				return fmt.Errorf("reading earlier batches: %w", err)
			}
			if prior != nil {
				return usageErr(fmt.Errorf("batch %s already has %d archive(s) for this same scope, account, task, date, filters and format that were created, may have been created, or are being sent; nothing was sent. Continue that batch with: cloudally-cli download submit --batch %s --resume --yes (or add --new-batch to start a second batch that creates every export again)",
					prior.BatchKey, n, prior.BatchKey))
			}
		}
		if rerunGuardPause > 0 {
			time.Sleep(rerunGuardPause)
		}
		return saveBatch(b)
	})
}
