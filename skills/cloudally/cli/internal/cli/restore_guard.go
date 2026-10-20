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

	"cloudally-pp-cli/internal/cliutil"
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
		c, err := liveClaim(ctx, g.db, id)
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
		live, err := liveClaim(ctx, tx, id)
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
