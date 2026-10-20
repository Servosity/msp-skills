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
	"sort"
	"strconv"
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
// account, task, the date (or --latest), the entity and sub-source filters,
// the format and both include flags. The timezone and --min-size are left
// out on purpose: a run that differs only in those still selects mostly the
// same archives, so it is refused too rather than quietly creating them
// twice.
func rerunIdentity(scope, account, task, onDate, format string, entities, subSources []string, includeDeleted, includeLegalHold bool) string {
	return strings.Join([]string{
		"v2", scope, strings.ToLower(strings.TrimSpace(account)), strings.TrimSpace(task),
		strings.ToLower(onDate), strings.ToLower(format),
		strings.Join(entities, ","), strings.Join(subSources, ","),
		fmt.Sprintf("deleted=%t", includeDeleted), fmt.Sprintf("hold=%t", includeLegalHold),
	}, "|")
}

// ownedRows counts the rows of a batch that exist, may exist, or are about
// to be sent. A refused create (FAILED), an archive another batch holds
// (SKIPPED) and a job that failed (JOB_FAILED) are not the batch's.
func ownedRows(b *downloadBatch) int {
	n := 0
	for _, r := range b.Rows {
		switch r.Status {
		case dlFailed, dlSkipped, dlJobFailed:
		default:
			n++
		}
	}
	return n
}

// mayExist reports whether a row's export was created or may have been:
// it was sent and not cleanly refused, or the portal accepted it.
func mayExist(r downloadRow) bool {
	switch r.Status {
	case dlSkipped, dlJobFailed, dlFailed, dlPlanned:
		return false
	case dlRequested, dlCreated, dlUnknown, dlMissing, dlEmpty:
		return true
	}
	return r.AcceptedAt != "" || r.ConfirmedAt != ""
}

// findRerunBatch returns the newest earlier batch for the same command (the
// same identity) that still owns rows. It catches a batch that was saved
// but never got as far as claiming anything in the download guard. A ledger
// it cannot read is an error, never a pass.
func findRerunBatch(nb *downloadBatch) (*downloadBatch, int, error) {
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
		if err != nil {
			return nil, 0, fmt.Errorf("batch ledger %s cannot be read (%v), so it cannot be checked for archives this run would create again; nothing was sent. Move it out of %s if it is not needed", filepath.Join(dir, e.Name()), err, dir)
		}
		n := 0
		if b.RerunKey != "" && b.RerunKey == nb.RerunKey {
			n = ownedRows(b)
		}
		if n > 0 && (best == nil || b.CreatedAt > best.CreatedAt) {
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
			prior, n, err := findRerunBatch(b)
			if err != nil {
				return usageErr(err)
			}
			if prior != nil {
				return usageErr(fmt.Errorf("batch %s is this same command and holds %d archive(s) that were created, may have been created, or are waiting to be sent; nothing was sent. Continue that batch with: cloudally-cli download submit --batch %s --resume --yes",
					prior.BatchKey, n, prior.BatchKey))
			}
		}
		if rerunGuardPause > 0 {
			time.Sleep(rerunGuardPause)
		}
		return saveBatch(b)
	})
}

// neverCreatedCommands rebuilds, per mailbox, a submit command limited to
// the rows a batch never created (never sent, or cleanly refused). An older
// ledger's FAILED row counts only when its error proves the request never
// left: that build wrote FAILED for timeouts and server errors too.
func neverCreatedCommands(b *downloadBatch) []string {
	legacy := b.IncludeDeleted == nil || b.IncludeLegalHold == nil
	var rows []downloadRow
	for _, r := range b.Rows {
		if mayExist(r) || r.Status == dlSkipped || (legacy && r.Status == dlFailed && !neverSent(r.Detail)) {
			continue
		}
		rows = append(rows, r)
	}
	return submitCommandsFor(b, rows)
}

// submitCommandsFor rebuilds, per mailbox, a submit command that creates
// exactly rows of b: the same account, task, date (or --latest), format and
// include flags, limited by --entities and --sub-sources. Include flags b
// did not record are left out.
func submitCommandsFor(b *downloadBatch, rows []downloadRow) []string {
	subs := map[string][]string{}
	var entities []string
	for _, r := range rows {
		if _, seen := subs[r.Entity]; !seen {
			entities = append(entities, r.Entity)
		}
		subs[r.Entity] = append(subs[r.Entity], r.SubSource)
	}
	sort.Strings(entities)
	base := "cloudally-cli download submit --account " + b.AccountID
	if b.Scope == scopeReseller {
		base += " --reseller"
	}
	base += " --task " + b.TaskID
	if strings.EqualFold(b.OnDate, latestLabel) {
		base += " --latest"
	} else {
		base += " --on-date " + b.OnDate
		if off := offsetHours(b.Timezone); off != "" {
			base += " --utc-offset " + off
		}
	}
	if f := strings.TrimSpace(b.Format); f != "" && f != downloadFormatZip {
		base += " --format " + f
	}
	if boolValue(b.IncludeDeleted) {
		base += " --include-deleted"
	}
	if boolValue(b.IncludeLegalHold) {
		base += " --include-legal-hold"
	}
	out := make([]string, 0, len(entities))
	for _, e := range entities {
		s := subs[e]
		sort.Strings(s)
		out = append(out, fmt.Sprintf("%s --entities %s --sub-sources %s --yes", base, e, strings.Join(s, ",")))
	}
	return out
}

// offsetHours turns a recorded zone label ("UTC-5", "UTC+5:30") back into a
// --utc-offset value, or "" for UTC.
func offsetHours(label string) string {
	rest, ok := strings.CutPrefix(strings.TrimSpace(label), "UTC")
	if !ok || rest == "" {
		return ""
	}
	sign := 1.0
	switch rest[0] {
	case '-':
		sign = -1
	case '+':
	default:
		return ""
	}
	hm := strings.SplitN(rest[1:], ":", 2)
	var h, m float64
	if _, err := fmt.Sscanf(hm[0], "%g", &h); err != nil {
		return ""
	}
	if len(hm) == 2 {
		if _, err := fmt.Sscanf(hm[1], "%g", &m); err != nil {
			return ""
		}
	}
	return strconv.FormatFloat(sign*(h+m/60), 'f', -1, 64)
}
