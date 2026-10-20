// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"cloudally-pp-cli/internal/cadate"
	"cloudally-pp-cli/internal/cliutil"
	"cloudally-pp-cli/internal/store"
)

// The restore guard is the machine-wide double-send lock for 'restore
// submit'. The ledger that holds batches, history, and --resume lives in a
// store per login (partner, one customer after use-account, one company
// login, or one raw token), so a ledger alone cannot see a restore another
// login, another token, or a racing process already sent. The guard is one
// small SQLite file in the CLI's state directory, shared by every login and
// token on this machine, that holds one live claim per restore identity:
// the backup task, the mailbox (lowercased), and the resolved backup
// timestamp, never the date text a run was given.
//
// A row is claimed inside an IMMEDIATE transaction right before its POST,
// and a partial UNIQUE index allows only one live claim per identity, so of
// two processes only one can claim a restore. A claim moves SENDING ->
// ACCEPTED, UNKNOWN, or REJECTED once the answer is known. REJECTED (the
// API refused it before running it) is not live, so that restore can be
// sent again. A SENDING claim whose owner process is gone becomes UNKNOWN.
// --new-batch marks the live claim superseded and records a new generation
// on top of it; nothing is deleted.
//
// Two resolved backups of one mailbox within sameBackupWindow of each other
// are treated as one backup: the partner calendar records a run's headline
// time while the task record's lastBackupDates can be per sub-source and
// seconds or minutes off, so one restore resolved two ways must still
// collide. Backups of one mailbox are hours apart.
//
// Every ledger already in the data directory is imported once, read-only,
// so restores recorded before the guard existed (or by a build without it)
// are refused too; see seedFromLedgers.
//
// The customer account id is recorded on each claim and shown in refusals
// but is not part of the identity: a raw token or a company login does not
// know it, and a task id already names exactly one customer's task, so
// leaving it out can only make more runs collide, never fewer.
const restoreGuardFile = "restore-guard.db"

// restoreGuardOwnersDir holds one lock file per running submit; the OS
// releases the lock when the process ends, which is how a claim's owner is
// proved gone.
const restoreGuardOwnersDir = "restore-guard-owners"

// restoreGuardBusyTimeout bounds how long a claim waits on another process.
const restoreGuardBusyTimeout = 30 * time.Second

// Claim states.
const (
	claimSending  = "SENDING"
	claimAccepted = "ACCEPTED"
	claimUnknown  = "UNKNOWN"
	claimRejected = "REJECTED"
)

var restoreGuardSchema = []string{
	`CREATE TABLE IF NOT EXISTS restore_claims (
		id            INTEGER PRIMARY KEY AUTOINCREMENT,
		task_id       TEXT NOT NULL,
		entity        TEXT NOT NULL,
		resolved_raw  INTEGER NOT NULL,
		generation    INTEGER NOT NULL DEFAULT 1,
		state         TEXT NOT NULL,
		superseded    INTEGER NOT NULL DEFAULT 0,
		superseded_by TEXT NOT NULL DEFAULT '',
		batch_id      TEXT NOT NULL DEFAULT '',
		login         TEXT NOT NULL DEFAULT '',
		account_id    TEXT NOT NULL DEFAULT '',
		scope         TEXT NOT NULL DEFAULT '',
		owner_run     TEXT NOT NULL DEFAULT '',
		owner_host    TEXT NOT NULL DEFAULT '',
		owner_pid     INTEGER NOT NULL DEFAULT 0,
		created_at    TEXT NOT NULL,
		updated_at    TEXT NOT NULL
	)`,
	`CREATE UNIQUE INDEX IF NOT EXISTS restore_claims_live ON restore_claims(task_id, entity, resolved_raw)
		WHERE superseded = 0 AND state IN ('SENDING', 'ACCEPTED', 'UNKNOWN')`,
	`CREATE INDEX IF NOT EXISTS restore_claims_identity ON restore_claims(task_id, entity, resolved_raw)`,
	// One row per ledger store already imported by seedFromLedgers.
	`CREATE TABLE IF NOT EXISTS seeded_stores (
		path       TEXT PRIMARY KEY,
		imported   INTEGER NOT NULL DEFAULT 0,
		seeded_at  TEXT NOT NULL
	)`,
}

// sameBackupWindow is how far apart two resolved backup times of one
// mailbox can be and still name the same backup.
const sameBackupWindow = 30 * time.Minute

// sameBackup reports whether two resolved backup times (yyyyMMddHHmmss)
// name the same backup: equal, or both valid and within sameBackupWindow.
func sameBackup(a, b int64) bool {
	if a == b {
		return true
	}
	ta, errA := cadate.ParseCompact(a)
	tb, errB := cadate.ParseCompact(b)
	if errA != nil || errB != nil {
		return false
	}
	d := ta.Sub(tb)
	if d < 0 {
		d = -d
	}
	return d <= sameBackupWindow
}

// restoreIdentity is what the guard allows to be sent once.
type restoreIdentity struct {
	TaskID   string
	Entity   string
	Resolved int64
}

func newRestoreIdentity(taskID, entity string, resolved int64) restoreIdentity {
	return restoreIdentity{
		TaskID:   strings.ToLower(strings.TrimSpace(taskID)),
		Entity:   strings.ToLower(strings.TrimSpace(entity)),
		Resolved: resolved,
	}
}

// guardClaim is one row of restore_claims.
type guardClaim struct {
	ID int64
	restoreIdentity
	Generation int
	State      string
	BatchID    string
	Login      string
	AccountID  string
	OwnerRun   string
	OwnerHost  string
	OwnerPID   int
}

// claimOwner describes the batch a claim is made for.
type claimOwner struct {
	batchID   string
	login     string
	accountID string
	scope     string
}

// errRestoreClaimed reports a restore another batch or process already
// holds.
type errRestoreClaimed struct{ claim guardClaim }

func (e errRestoreClaimed) Error() string {
	return fmt.Sprintf("restore already %s in batch %s (login %s) on this machine; not sending it again",
		claimStateText(e.claim.State), e.claim.BatchID, e.claim.Login)
}

func claimStateText(state string) string {
	switch state {
	case claimAccepted:
		return "accepted"
	case claimSending:
		return "being sent by another running submit"
	default:
		return "sent with an unknown outcome"
	}
}

// restoreGuard is an open guard plus this run's owner lock.
type restoreGuard struct {
	db        *sql.DB
	path      string
	ownersDir string
	run       string
	host      string
	pid       int
	stopOwner func()
}

// openRestoreGuard opens (creating if needed) the guard file with mode 0600
// and takes this run's owner lock. Close releases both.
func openRestoreGuard(ctx context.Context) (*restoreGuard, error) {
	dir, err := cliutil.StateDir()
	if err != nil {
		return nil, fmt.Errorf("locating the state directory: %w", err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("creating %s: %w", dir, err)
	}
	path := filepath.Join(dir, restoreGuardFile)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600) // #nosec G304 -- fixed name in the CLI state dir.
	if err != nil {
		return nil, fmt.Errorf("opening %s: %w", path, err)
	}
	_ = f.Close()
	db, err := sql.Open("sqlite", path+"?_txlock=immediate&_pragma=busy_timeout(10000)&_pragma=journal_mode(WAL)&_pragma=synchronous(FULL)")
	if err != nil {
		return nil, fmt.Errorf("opening %s: %w", path, err)
	}
	g := &restoreGuard{db: db, path: path, ownersDir: filepath.Join(dir, restoreGuardOwnersDir), pid: os.Getpid()}
	g.host, _ = os.Hostname()
	if err := guardRetryBusy(ctx, func() error { return db.PingContext(ctx) }); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("opening %s: %w", path, err)
	}
	for _, sidecar := range []string{path + "-wal", path + "-shm"} {
		_ = os.Chmod(sidecar, 0o600)
	}
	if err := g.inTx(ctx, func(tx *sql.Tx) error {
		for _, stmt := range restoreGuardSchema {
			if _, err := tx.ExecContext(ctx, stmt); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("preparing %s: %w", path, err)
	}
	if err := g.holdOwnerLock(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return g, nil
}

// holdOwnerLock takes an OS lock on a file named for this run and keeps it
// until Close. Another process that can take that lock knows this run has
// ended; one that cannot knows it is still sending.
func (g *restoreGuard) holdOwnerLock() error {
	buf := make([]byte, 8)
	_, _ = rand.Read(buf)
	g.run = fmt.Sprintf("%d-%s", g.pid, hex.EncodeToString(buf))
	if err := os.MkdirAll(g.ownersDir, 0o700); err != nil {
		return fmt.Errorf("creating %s: %w", g.ownersDir, err)
	}
	lockPath := filepath.Join(g.ownersDir, g.run)
	locked := make(chan error, 1)
	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		err := cliutil.WithFileLock(lockPath, func() error {
			locked <- nil
			<-done
			return nil
		})
		if err != nil {
			select {
			case locked <- err:
			default:
			}
		}
	}()
	if err := <-locked; err != nil {
		wg.Wait()
		return fmt.Errorf("taking the restore guard owner lock: %w", err)
	}
	g.stopOwner = func() {
		close(done)
		wg.Wait()
		_ = os.Remove(lockPath + ".lock")
	}
	return nil
}

// Close releases the owner lock and the database.
func (g *restoreGuard) Close() {
	if g == nil {
		return
	}
	if g.stopOwner != nil {
		g.stopOwner()
		g.stopOwner = nil
	}
	_ = g.db.Close()
}

// ownerAlive reports whether the run that made a claim may still be
// sending. A run is proved gone only when its owner lock can be taken: the
// OS drops that lock when the process ends, however it ended, and a reused
// pid cannot hold it. Anything that cannot be checked counts as alive. The
// host name is recorded for people, not used here: macOS can change it
// when the network changes, which would strand a crashed run's claim.
func (g *restoreGuard) ownerAlive(c guardClaim) bool {
	switch {
	case c.OwnerRun == g.run:
		return true
	case c.OwnerRun == "":
		return false
	case strings.ContainsAny(c.OwnerRun, `/\.`):
		return true
	}
	lockPath := filepath.Join(g.ownersDir, c.OwnerRun)
	acquired, err := cliutil.TryWithFileLock(lockPath, func() error { return nil })
	if err != nil || !acquired {
		return true
	}
	_ = os.Remove(lockPath + ".lock")
	return false
}

// inTx runs fn in one IMMEDIATE transaction, retrying while another process
// holds the write lock.
func (g *restoreGuard) inTx(ctx context.Context, fn func(*sql.Tx) error) error {
	return guardRetryBusy(ctx, func() error {
		tx, err := g.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if err := fn(tx); err != nil {
			_ = tx.Rollback()
			return err
		}
		return tx.Commit()
	})
}

func guardRetryBusy(ctx context.Context, op func() error) error {
	deadline := time.Now().Add(restoreGuardBusyTimeout)
	backoff := 5 * time.Millisecond
	for {
		err := op()
		if err == nil || !guardBusy(err) || time.Now().After(deadline) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}
		if backoff < 200*time.Millisecond {
			backoff *= 2
		}
	}
}

func guardBusy(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "SQLITE_BUSY") || strings.Contains(msg, "SQLITE_LOCKED") || strings.Contains(msg, "database is locked")
}

const guardClaimColumns = `id, task_id, entity, resolved_raw, generation, state, batch_id, login, account_id, owner_run, owner_host, owner_pid`

type rowScanner interface{ Scan(dest ...any) error }

func scanGuardClaim(row rowScanner) (guardClaim, error) {
	var c guardClaim
	err := row.Scan(&c.ID, &c.TaskID, &c.Entity, &c.Resolved, &c.Generation, &c.State, &c.BatchID, &c.Login, &c.AccountID, &c.OwnerRun, &c.OwnerHost, &c.OwnerPID)
	return c, err
}

type queryer interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// liveClaim returns the identity's live claim, or nil.
func liveClaim(ctx context.Context, q queryer, id restoreIdentity) (*guardClaim, error) {
	c, err := scanGuardClaim(q.QueryRowContext(ctx, `SELECT `+guardClaimColumns+` FROM restore_claims
		WHERE task_id = ? AND entity = ? AND resolved_raw = ? AND superseded = 0 AND state IN (?, ?, ?)`,
		id.TaskID, id.Entity, id.Resolved, claimSending, claimAccepted, claimUnknown))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// liveClaimNear returns the live claim that names the same backup of the
// same mailbox on the same task: the exact identity if one is live,
// otherwise the nearest live claim within sameBackupWindow, or nil.
func liveClaimNear(ctx context.Context, q queryer, id restoreIdentity) (*guardClaim, error) {
	rows, err := q.QueryContext(ctx, `SELECT `+guardClaimColumns+` FROM restore_claims
		WHERE task_id = ? AND entity = ? AND superseded = 0 AND state IN (?, ?, ?)`,
		id.TaskID, id.Entity, claimSending, claimAccepted, claimUnknown)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var best *guardClaim
	var bestGap time.Duration
	for rows.Next() {
		c, err := scanGuardClaim(rows)
		if err != nil {
			return nil, err
		}
		if !sameBackup(c.Resolved, id.Resolved) {
			continue
		}
		gap := time.Duration(0)
		if c.Resolved != id.Resolved {
			ta, _ := cadate.ParseCompact(c.Resolved)
			tb, _ := cadate.ParseCompact(id.Resolved)
			if gap = ta.Sub(tb); gap < 0 {
				gap = -gap
			}
			gap++
		}
		if best == nil || gap < bestGap {
			cc := c
			best, bestGap = &cc, gap
		}
	}
	return best, rows.Err()
}

// blocks reports whether a live claim stops owner from sending: anything
// accepted, unknown, or in flight, except an UNKNOWN restore of the same
// batch that --retry-unknown asked for. --new-batch overrides all but a
// send that is in flight right now.
func (g *restoreGuard) blocks(c guardClaim, owner claimOwner, retryUnknown, newBatch bool) bool {
	state := c.State
	if state == claimSending && !g.ownerAlive(c) {
		state = claimUnknown
	}
	switch {
	case state == claimSending:
		return true
	case state == claimUnknown && retryUnknown && owner.batchID != "" && c.BatchID == owner.batchID:
		return false
	case newBatch:
		return false
	}
	return true
}

// conflicts returns the live claims that would stop these identities from
// being sent, without changing anything. It is the up-front check that
// refuses a run before any request leaves.
func (g *restoreGuard) conflicts(ctx context.Context, ids []restoreIdentity, owner claimOwner, retryUnknown, newBatch bool) (map[restoreIdentity]guardClaim, error) {
	out := map[restoreIdentity]guardClaim{}
	for _, id := range ids {
		c, err := liveClaimNear(ctx, g.db, id)
		if err != nil {
			return nil, err
		}
		if c != nil && g.blocks(*c, owner, retryUnknown, newBatch) {
			if c.State == claimSending && !g.ownerAlive(*c) {
				c.State = claimUnknown
			}
			out[id] = *c
		}
	}
	return out, nil
}

// inFlight reports whether a live run is sending this identity for batchID
// right now.
func (g *restoreGuard) inFlight(ctx context.Context, id restoreIdentity, batchID string) bool {
	c, err := liveClaim(ctx, g.db, id)
	if err != nil {
		return true
	}
	return c != nil && c.State == claimSending && c.BatchID == batchID && g.ownerAlive(*c)
}

// claim takes the identity for owner right before its POST. It returns the
// claim id to settle afterwards, or errRestoreClaimed with the claim that
// holds it. A SENDING claim whose owner is gone is first marked UNKNOWN.
func (g *restoreGuard) claim(ctx context.Context, id restoreIdentity, owner claimOwner, retryUnknown, newBatch bool) (int64, error) {
	var claimID int64
	err := g.inTx(ctx, func(tx *sql.Tx) error {
		now := isoUTC(time.Now())
		live, err := liveClaimNear(ctx, tx, id)
		if err != nil {
			return err
		}
		generation := 1
		if live != nil {
			if live.State == claimSending && !g.ownerAlive(*live) {
				if _, err := tx.ExecContext(ctx, `UPDATE restore_claims SET state = ?, updated_at = ? WHERE id = ?`, claimUnknown, now, live.ID); err != nil {
					return err
				}
				live.State = claimUnknown
			}
			if g.blocks(*live, owner, retryUnknown, newBatch) {
				return errRestoreClaimed{claim: *live}
			}
			if live.State == claimUnknown && retryUnknown && live.BatchID == owner.batchID {
				claimID = live.ID
				_, err := tx.ExecContext(ctx, `UPDATE restore_claims SET state = ?, owner_run = ?, owner_host = ?, owner_pid = ?, updated_at = ? WHERE id = ?`,
					claimSending, g.run, g.host, g.pid, now, live.ID)
				return err
			}
			if _, err := tx.ExecContext(ctx, `UPDATE restore_claims SET superseded = 1, superseded_by = ?, updated_at = ? WHERE id = ?`, owner.batchID, now, live.ID); err != nil {
				return err
			}
			generation = live.Generation + 1
		} else if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(generation), 1) FROM restore_claims WHERE task_id = ? AND entity = ? AND resolved_raw = ?`,
			id.TaskID, id.Entity, id.Resolved).Scan(&generation); err != nil {
			return err
		}
		res, err := tx.ExecContext(ctx, `INSERT INTO restore_claims
			(task_id, entity, resolved_raw, generation, state, batch_id, login, account_id, scope, owner_run, owner_host, owner_pid, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			id.TaskID, id.Entity, id.Resolved, generation, claimSending, owner.batchID, owner.login, owner.accountID, owner.scope, g.run, g.host, g.pid, now, now)
		if err != nil {
			return err
		}
		claimID, err = res.LastInsertId()
		return err
	})
	return claimID, err
}

// settle records what the POST for a claim returned: SUBMITTED is
// ACCEPTED, FAILED (refused before it ran) is REJECTED and frees the
// identity, anything else stays UNKNOWN.
func (g *restoreGuard) settle(ctx context.Context, claimID int64, ledgerStatus string) error {
	state := claimUnknown
	switch ledgerStatus {
	case jobSubmitted:
		state = claimAccepted
	case jobFailed:
		state = claimRejected
	}
	return g.inTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE restore_claims SET state = ?, updated_at = ? WHERE id = ? AND owner_run = ?`,
			state, isoUTC(time.Now()), claimID, g.run)
		return err
	})
}

// sentLedgerRow is one ledger row seedFromLedgers imports.
type sentLedgerRow struct {
	batchID, taskID, scope, accountID, authScope string
	entity                                       string
	resolved                                     int64
	outcome, rowTaskID                           string
}

// ledgerDataDir is the directory every login's ledger store lives in, as
// defaultDBPath resolves it.
func ledgerDataDir() string {
	if dir, err := cliutil.DataDir(); err == nil {
		return dir
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".local", "share", restoreCLIName)
	}
	return "."
}

// seedFromLedgers imports, once per store file, every restore any ledger in
// dir recorded as sent (SUBMITTED, SUBMITTING, or UNKNOWN, or carrying a
// RESTORE task id) as an ACCEPTED or UNKNOWN claim, so a restore sent
// before this guard existed, or by a build without it, is still refused.
// Stores are opened read-only and never migrated. A store that cannot be
// read fails the run closed: nothing is sent while a ledger that may hold
// sent restores is unreadable.
func (g *restoreGuard) seedFromLedgers(ctx context.Context, dir string) error {
	paths, err := filepath.Glob(filepath.Join(dir, "data-*.db"))
	if err != nil {
		return fmt.Errorf("listing ledgers in %s: %w", dir, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "data.db")); err == nil {
		paths = append(paths, filepath.Join(dir, "data.db"))
	}
	for _, path := range paths {
		if abs, err := filepath.Abs(path); err == nil {
			path = abs
		}
		var seen int
		if err := g.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM seeded_stores WHERE path = ?`, path).Scan(&seen); err != nil {
			return fmt.Errorf("reading the restore guard %s: %w", g.path, err)
		}
		if seen > 0 {
			continue
		}
		rows, err := readSentLedgerRows(ctx, path)
		if err != nil {
			return fmt.Errorf("the restore ledger %s cannot be read, so the restores it records cannot be checked (%w); nothing was sent. Fix or move that file, then run again", path, err)
		}
		if err := g.inTx(ctx, func(tx *sql.Tx) error {
			if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM seeded_stores WHERE path = ?`, path).Scan(&seen); err != nil || seen > 0 {
				return err
			}
			now := isoUTC(time.Now())
			imported := 0
			for _, r := range rows {
				id := newRestoreIdentity(r.taskID, r.entity, r.resolved)
				live, err := liveClaimNear(ctx, tx, id)
				if err != nil {
					return err
				}
				if live != nil {
					continue
				}
				state := claimUnknown
				if r.outcome == jobSubmitted || strings.TrimSpace(r.rowTaskID) != "" {
					state = claimAccepted
				}
				if _, err := tx.ExecContext(ctx, `INSERT INTO restore_claims
					(task_id, entity, resolved_raw, generation, state, batch_id, login, account_id, scope, created_at, updated_at)
					VALUES (?, ?, ?, 1, ?, ?, ?, ?, ?, ?, ?)`,
					id.TaskID, id.Entity, id.Resolved, state, r.batchID, historyAccountLabel(r.accountID, r.authScope), r.accountID, r.scope, now, now); err != nil {
					return err
				}
				imported++
			}
			_, err := tx.ExecContext(ctx, `INSERT INTO seeded_stores (path, imported, seeded_at) VALUES (?, ?, ?)`, path, imported, now)
			return err
		}); err != nil {
			return fmt.Errorf("seeding the restore guard %s from %s: %w; nothing was sent", g.path, path, err)
		}
	}
	return nil
}

// readSentLedgerRows reads one store's restore rows that were sent, through
// a read-only open that never migrates it. A store without restore tables
// holds none.
func readSentLedgerRows(ctx context.Context, path string) ([]sentLedgerRow, error) {
	st, err := store.OpenReadOnlyContext(ctx, path)
	if err != nil {
		return nil, err
	}
	defer st.Close()
	db := st.DB()
	var tables int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name IN ('restore_jobs', 'restore_batches')`).Scan(&tables); err != nil {
		return nil, err
	}
	if tables < 2 {
		return nil, nil
	}
	authScope := "b.auth_scope"
	if !ledgerHasColumn(db, "restore_batches", "auth_scope") {
		authScope = "''"
	}
	outcome := submitStatusExpr(db, "j.")
	q, err := db.QueryContext(ctx, `SELECT b.batch_id, b.task_id, b.scope, b.account_id, `+authScope+`, j.entity, j.resolved_raw, `+outcome+`, COALESCE(j.task_id, '')
		FROM restore_jobs j JOIN restore_batches b ON b.batch_id = j.batch_id
		WHERE `+outcome+` IN (?, ?, ?) OR COALESCE(j.task_id, '') <> ''`, jobSubmitted, jobSubmitting, jobUnknown)
	if err != nil {
		return nil, err
	}
	defer q.Close()
	var out []sentLedgerRow
	for q.Next() {
		var r sentLedgerRow
		if err := q.Scan(&r.batchID, &r.taskID, &r.scope, &r.accountID, &r.authScope, &r.entity, &r.resolved, &r.outcome, &r.rowTaskID); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, q.Err()
}
