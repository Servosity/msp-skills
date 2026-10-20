// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"cloudally-pp-cli/internal/cliutil"
)

// The download guard is the double-create lock for 'download submit', the
// same shape as the restore guard (restore_guard.go). The JSON batch
// ledgers keep each batch's progress and drive --resume, but a ledger only
// knows its own batch. The guard is one small SQLite file in the CLI's
// state directory that holds one live claim per archive identity:
//
//	account, task, mailbox (lowercased), sub-source, snapshot, format,
//	include_deleted, include_legal_hold
//
// so a zip and a PST of the same snapshot, or the same snapshot with and
// without legal-hold items, are different archives and never block each
// other, while the same archive can be claimed only once.
//
// A row is claimed inside an IMMEDIATE transaction right before its create,
// and a partial UNIQUE index allows one live claim per identity. States:
//
//   - IN_FLIGHT: the create is being sent by a run that holds its owner
//     lock. A claim whose owner process is gone becomes UNKNOWN.
//   - UNKNOWN: the create may have reached the portal; held until its own
//     batch confirms it, or retries it with --retry-unknown.
//   - CREATED: the portal accepted the create. job_id is pinned once the
//     batch confirms which portal job is this archive.
//   - JOB_FAILED: the confirmed job ended in failure. Not live:
//     the identity is free, for its own batch only, to be created once more.
//   - REJECTED: the portal refused the create before running it. Not live.
//
// --new-batch marks the live claim superseded and claims on top of it; it
// is the only override, and nothing is ever deleted.
//
// Every JSON ledger in the batch folder, including ones written before the
// guard existed, is imported once, read-only (seedFromLedgers). A ledger
// that did not record its include flags is imported with them unknown
// (-1), which matches any value.
const (
	downloadGuardFile      = "download-guard.db"
	downloadGuardOwnersDir = "download-guard-owners"
)

// Download claim states.
const (
	dgInFlight  = "IN_FLIGHT"
	dgUnknown   = "UNKNOWN"
	dgCreated   = "CREATED"
	dgJobFailed = "JOB_FAILED"
	dgRejected  = "REJECTED"
)

// flagNotRecorded marks an include flag a ledger did not record.
const flagNotRecorded = -1

var downloadGuardSchema = []string{
	`CREATE TABLE IF NOT EXISTS download_claims (
		id                 INTEGER PRIMARY KEY AUTOINCREMENT,
		account_id         TEXT NOT NULL,
		task_id            TEXT NOT NULL,
		entity             TEXT NOT NULL,
		sub_source         TEXT NOT NULL,
		snapshot           INTEGER NOT NULL,
		format             TEXT NOT NULL,
		include_deleted    INTEGER NOT NULL,
		include_legal_hold INTEGER NOT NULL,
		generation         INTEGER NOT NULL DEFAULT 1,
		state              TEXT NOT NULL,
		superseded         INTEGER NOT NULL DEFAULT 0,
		superseded_by      TEXT NOT NULL DEFAULT '',
		batch_key          TEXT NOT NULL DEFAULT '',
		job_id             TEXT NOT NULL DEFAULT '',
		label              TEXT NOT NULL DEFAULT '',
		scope              TEXT NOT NULL DEFAULT '',
		owner_run          TEXT NOT NULL DEFAULT '',
		owner_host         TEXT NOT NULL DEFAULT '',
		owner_pid          INTEGER NOT NULL DEFAULT 0,
		created_at         TEXT NOT NULL,
		updated_at         TEXT NOT NULL
	)`,
	`CREATE UNIQUE INDEX IF NOT EXISTS download_claims_live ON download_claims(
		account_id, task_id, entity, sub_source, snapshot, format, include_deleted, include_legal_hold)
		WHERE superseded = 0 AND state IN ('IN_FLIGHT', 'UNKNOWN', 'CREATED')`,
	`CREATE INDEX IF NOT EXISTS download_claims_archive ON download_claims(account_id, task_id, entity, sub_source, snapshot)`,
	`CREATE INDEX IF NOT EXISTS download_claims_job ON download_claims(job_id)`,
	// One row per ledger file already imported by seedFromLedgers.
	`CREATE TABLE IF NOT EXISTS seeded_ledgers (
		path      TEXT PRIMARY KEY,
		imported  INTEGER NOT NULL DEFAULT 0,
		seeded_at TEXT NOT NULL
	)`,
}

// downloadIdentity is one archive: what the guard allows to be created once.
type downloadIdentity struct {
	Account, Task, Entity, SubSource string
	Snapshot                         int64
	Format                           string
	IncludeDeleted, IncludeLegalHold int
}

func flagValue(p *bool) int {
	switch {
	case p == nil:
		return flagNotRecorded
	case *p:
		return 1
	}
	return 0
}

// rowIdentity is the archive one batch row asks for.
func rowIdentity(b *downloadBatch, r *downloadRow) downloadIdentity {
	format := strings.ToLower(strings.TrimSpace(b.Format))
	if format == "" {
		format = downloadFormatZip
	}
	return downloadIdentity{
		Account:          strings.ToLower(strings.TrimSpace(b.AccountID)),
		Task:             strings.ToLower(strings.TrimSpace(b.TaskID)),
		Entity:           strings.ToLower(strings.TrimSpace(r.Entity)),
		SubSource:        strings.ToUpper(strings.TrimSpace(r.SubSource)),
		Snapshot:         r.SnapshotRaw,
		Format:           format,
		IncludeDeleted:   flagValue(b.IncludeDeleted),
		IncludeLegalHold: flagValue(b.IncludeLegalHold),
	}
}

// identityWhere matches one identity; an include flag that was not recorded
// (-1), on either side, matches any value.
const identityWhere = `account_id = ? AND task_id = ? AND entity = ? AND sub_source = ? AND snapshot = ? AND format = ?
	AND (include_deleted = ? OR include_deleted = -1 OR ? = -1)
	AND (include_legal_hold = ? OR include_legal_hold = -1 OR ? = -1)`

func (id downloadIdentity) args() []any {
	return []any{id.Account, id.Task, id.Entity, id.SubSource, id.Snapshot, id.Format,
		id.IncludeDeleted, id.IncludeDeleted, id.IncludeLegalHold, id.IncludeLegalHold}
}

// downloadClaim is one row of download_claims.
type downloadClaim struct {
	ID int64
	downloadIdentity
	Generation int
	State      string
	BatchKey   string
	JobID      string
	Label      string
	OwnerRun   string
}

const downloadClaimColumns = `id, account_id, task_id, entity, sub_source, snapshot, format, include_deleted, include_legal_hold,
	generation, state, batch_key, job_id, label, owner_run`

func scanDownloadClaim(row rowScanner) (downloadClaim, error) {
	var c downloadClaim
	err := row.Scan(&c.ID, &c.Account, &c.Task, &c.Entity, &c.SubSource, &c.Snapshot, &c.Format, &c.IncludeDeleted, &c.IncludeLegalHold,
		&c.Generation, &c.State, &c.BatchKey, &c.JobID, &c.Label, &c.OwnerRun)
	return c, err
}

// errDownloadClaimed reports an archive another batch or run already holds.
type errDownloadClaimed struct{ claim downloadClaim }

func (e errDownloadClaimed) Error() string {
	return fmt.Sprintf("this archive is already %s by batch %s in this state directory; not creating it again",
		downloadClaimText(e.claim.State), e.claim.BatchKey)
}

func downloadClaimText(state string) string {
	switch state {
	case dgCreated:
		return "created"
	case dgInFlight:
		return "being created by another running submit"
	case dgJobFailed:
		return "waiting to be created again (its job failed)"
	default:
		return "sent with an unknown outcome"
	}
}

// downloadGuard is an open guard plus this run's owner lock. The database
// handle, the owner lock and the transaction helpers are the restore
// guard's own.
type downloadGuard struct{ *restoreGuard }

// downloadClaimPause is a test hook: how long a submit waits after its
// up-front check and before it claims anything, to widen the window two
// racing submits would otherwise rarely hit.
var downloadClaimPause time.Duration

// openDownloadGuard opens (creating if needed) the guard file with mode
// 0600 and takes this run's owner lock. Close releases both.
func openDownloadGuard(ctx context.Context) (*downloadGuard, error) {
	dir, err := cliutil.StateDir()
	if err != nil {
		return nil, fmt.Errorf("locating the state directory: %w", err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("creating %s: %w", dir, err)
	}
	path := filepath.Join(dir, downloadGuardFile)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600) // #nosec G304 -- fixed name in the CLI state dir.
	if err != nil {
		return nil, fmt.Errorf("opening %s: %w", path, err)
	}
	_ = f.Close()
	db, err := sql.Open("sqlite", path+"?_txlock=immediate&_pragma=busy_timeout(10000)&_pragma=journal_mode(WAL)&_pragma=synchronous(FULL)")
	if err != nil {
		return nil, fmt.Errorf("opening %s: %w", path, err)
	}
	g := &restoreGuard{db: db, path: path, ownersDir: filepath.Join(dir, downloadGuardOwnersDir), pid: os.Getpid()}
	g.host, _ = os.Hostname()
	if err := guardRetryBusy(ctx, func() error { return db.PingContext(ctx) }); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("opening %s: %w", path, err)
	}
	for _, sidecar := range []string{path + "-wal", path + "-shm"} {
		_ = os.Chmod(sidecar, 0o600)
	}
	if err := g.inTx(ctx, func(tx *sql.Tx) error {
		for _, stmt := range downloadGuardSchema {
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
	dg := &downloadGuard{g}
	// A run killed between a create and recording its answer leaves its
	// claim IN_FLIGHT. Every opener (submit, start, fetch) turns those into
	// UNKNOWN first, so later confirmations and failures land on them.
	if err := dg.recoverAbandoned(ctx); err != nil {
		dg.Close()
		return nil, fmt.Errorf("recovering abandoned claims in %s: %w", path, err)
	}
	return dg, nil
}

// liveClaims returns the identity's live claims, other batches' first. With
// an unrecorded include flag on either side there can be more than one.
func (g *downloadGuard) liveClaims(ctx context.Context, q queryer, id downloadIdentity, batchKey string) ([]downloadClaim, error) {
	args := append(id.args(), batchKey)
	rows, err := q.QueryContext(ctx, `SELECT `+downloadClaimColumns+` FROM download_claims
		WHERE `+identityWhere+` AND superseded = 0 AND state IN ('IN_FLIGHT', 'UNKNOWN', 'CREATED')
		ORDER BY CASE WHEN batch_key = ? THEN 1 ELSE 0 END, id DESC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []downloadClaim
	for rows.Next() {
		c, err := scanDownloadClaim(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// lastClaim returns the identity's most recent claim that was not
// superseded, live or not, or nil.
func (g *downloadGuard) lastClaim(ctx context.Context, q queryer, id downloadIdentity) (*downloadClaim, error) {
	c, err := scanDownloadClaim(q.QueryRowContext(ctx, `SELECT `+downloadClaimColumns+` FROM download_claims
		WHERE `+identityWhere+` AND superseded = 0 ORDER BY id DESC LIMIT 1`, id.args()...))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// effective reports a claim's state with a dead owner's IN_FLIGHT read as
// UNKNOWN.
func (g *downloadGuard) effective(c downloadClaim) string {
	if c.State == dgInFlight && !g.ownerAlive(guardClaim{OwnerRun: c.OwnerRun}) {
		return dgUnknown
	}
	return c.State
}

// conflict returns the claim that stops batchKey from creating id, without
// changing anything, or nil. It is the up-front check that refuses a run
// before any request leaves; claim makes the same decision atomically.
func (g *downloadGuard) conflict(ctx context.Context, id downloadIdentity, batchKey string) (*downloadClaim, error) {
	live, err := g.liveClaims(ctx, g.db, id, batchKey)
	if err != nil {
		return nil, err
	}
	for _, c := range live {
		if c.BatchKey != batchKey {
			c.State = g.effective(c)
			return &c, nil
		}
	}
	if len(live) == 0 {
		last, err := g.lastClaim(ctx, g.db, id)
		if err != nil {
			return nil, err
		}
		if last != nil && last.State == dgJobFailed && last.BatchKey != batchKey {
			return last, nil
		}
	}
	return nil, nil
}

// sameFlags reports whether two identities name the same include flags
// exactly, an unrecorded flag matching only another unrecorded one.
func sameFlags(a, b downloadIdentity) bool {
	return a.IncludeDeleted == b.IncludeDeleted && a.IncludeLegalHold == b.IncludeLegalHold
}

// downloadOwner is the batch a claim is made for.
type downloadOwner struct {
	batchKey, label, scope string
}

// claim takes the identity for owner right before its create and returns
// the claim id to settle afterwards, or errDownloadClaimed with the claim
// that holds it. In one IMMEDIATE transaction:
//
//   - a claim IN_FLIGHT under a live owner always blocks;
//   - --new-batch supersedes any other live claim (the only override);
//   - the batch's own claim is reused when its create was accepted but
//     never confirmed (a MISSING row sent again), or when it is UNKNOWN and
//     --retry-unknown was given; a confirmed one is never created again;
//   - any other batch's live claim blocks, and so does another batch's
//     JOB_FAILED claim: that archive is its batch's to create again.
//
// reused is true when the claim was the batch's own UNKNOWN or accepted
// one: an earlier create of this archive may exist, so nothing that
// happens to this resend may release it (runDownloadBatch). superseded
// lists the claims --new-batch superseded for this create; settle puts
// them back if the create is refused or never sent.
func (g *downloadGuard) claim(ctx context.Context, id downloadIdentity, owner downloadOwner, retryUnknown, newBatch bool) (claimID int64, reused bool, superseded []int64, err error) {
	err = g.inTx(ctx, func(tx *sql.Tx) error {
		// A busy retry runs this again from the start.
		claimID, reused, superseded = 0, false, nil
		now := isoUTC(time.Now())
		live, err := g.liveClaims(ctx, tx, id, owner.batchKey)
		if err != nil {
			return err
		}
		generation := 1
		var reuse *downloadClaim
		for i := range live {
			c := live[i]
			if c.State == dgInFlight && !g.ownerAlive(guardClaim{OwnerRun: c.OwnerRun}) {
				if _, err := tx.ExecContext(ctx, `UPDATE download_claims SET state = ?, updated_at = ? WHERE id = ?`, dgUnknown, now, c.ID); err != nil {
					return err
				}
				c.State = dgUnknown
			}
			switch {
			case c.State == dgInFlight:
				return errDownloadClaimed{claim: c}
			case newBatch && !sameFlags(c.downloadIdentity, id):
				// A claim whose include flags were never recorded may be a
				// different archive from this one: it stays live (and keeps
				// blocking the archives it may be) and does not block this.
			case newBatch:
				if _, err := tx.ExecContext(ctx, `UPDATE download_claims SET superseded = 1, superseded_by = ?, updated_at = ? WHERE id = ?`, owner.batchKey, now, c.ID); err != nil {
					return err
				}
				superseded = append(superseded, c.ID)
				generation = max(generation, c.Generation+1)
			case c.BatchKey != owner.batchKey:
				return errDownloadClaimed{claim: c}
			case c.State == dgCreated && c.JobID == "", c.State == dgUnknown && retryUnknown:
				reuse = &live[i]
			default:
				return errDownloadClaimed{claim: c}
			}
		}
		if reuse != nil {
			claimID, reused = reuse.ID, true
			_, err := tx.ExecContext(ctx, `UPDATE download_claims SET state = ?, label = ?, job_id = '', owner_run = ?, owner_host = ?, owner_pid = ?, updated_at = ? WHERE id = ?`,
				dgInFlight, owner.label, g.run, g.host, g.pid, now, reuse.ID)
			return err
		}
		if len(live) == 0 {
			last, err := g.lastClaim(ctx, tx, id)
			if err != nil {
				return err
			}
			if last != nil {
				if last.State == dgJobFailed && last.BatchKey != owner.batchKey && !newBatch {
					return errDownloadClaimed{claim: *last}
				}
				generation = last.Generation + 1
			}
		}
		res, err := tx.ExecContext(ctx, `INSERT INTO download_claims
			(account_id, task_id, entity, sub_source, snapshot, format, include_deleted, include_legal_hold,
			 generation, state, batch_key, label, scope, owner_run, owner_host, owner_pid, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			id.Account, id.Task, id.Entity, id.SubSource, id.Snapshot, id.Format, id.IncludeDeleted, id.IncludeLegalHold,
			generation, dgInFlight, owner.batchKey, owner.label, owner.scope, g.run, g.host, g.pid, now, now)
		if err != nil {
			return err
		}
		claimID, err = res.LastInsertId()
		return err
	})
	if err != nil {
		superseded = nil
	}
	return claimID, reused, superseded, err
}

// settle records what the create for a claim returned: accepted is
// CREATED, refused before it ran (or never sent) is REJECTED, anything else
// stays UNKNOWN.
//
// A REJECTED --new-batch create puts back, in the same transaction, the
// claims it superseded (restore), exactly as they were: their state, job
// and batch were never changed, only the superseded mark. Otherwise the
// archive the original batch holds would be free, and a later plain run
// would create it a second time. An accepted or UNKNOWN create keeps the
// identity held by its own claim, so nothing is put back.
func (g *downloadGuard) settle(ctx context.Context, claimID int64, rowStatus string, accepted bool, restore []int64) error {
	state := dgUnknown
	switch {
	case rowStatus == dlFailed:
		state = dgRejected
	case accepted:
		state = dgCreated
	}
	return g.inTx(ctx, func(tx *sql.Tx) error {
		now := isoUTC(time.Now())
		res, err := tx.ExecContext(ctx, `UPDATE download_claims SET state = ?, updated_at = ? WHERE id = ? AND owner_run = ?`,
			state, now, claimID, g.run)
		if err != nil {
			return err
		}
		if n, err := res.RowsAffected(); err != nil || n != 1 {
			return fmt.Errorf("the download guard no longer holds claim %d for this run (%d rows); its answer was not recorded there", claimID, n)
		}
		if state != dgRejected {
			return nil
		}
		for _, id := range restore {
			if _, err := tx.ExecContext(ctx, `UPDATE download_claims SET superseded = 0, superseded_by = '', updated_at = ?
				WHERE id = ? AND superseded = 1 AND superseded_by = (SELECT batch_key FROM download_claims WHERE id = ?)`,
				now, id, claimID); err != nil {
				return err
			}
		}
		return nil
	})
}

// recoverAbandoned turns every IN_FLIGHT claim whose run has ended into
// UNKNOWN, so a later confirmation or failure can be recorded against it.
// claim does the same for one identity; this covers the rest before a
// reconcile reads them.
func (g *downloadGuard) recoverAbandoned(ctx context.Context) error {
	return g.inTx(ctx, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `SELECT id, owner_run FROM download_claims WHERE state = ? AND superseded = 0`, dgInFlight)
		if err != nil {
			return err
		}
		var dead []int64
		for rows.Next() {
			var id int64
			var owner string
			if err := rows.Scan(&id, &owner); err != nil {
				rows.Close()
				return err
			}
			if !g.ownerAlive(guardClaim{OwnerRun: owner}) {
				dead = append(dead, id)
			}
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		now := isoUTC(time.Now())
		for _, id := range dead {
			if _, err := tx.ExecContext(ctx, `UPDATE download_claims SET state = ?, updated_at = ? WHERE id = ? AND state = ?`, dgUnknown, now, id, dgInFlight); err != nil {
				return err
			}
		}
		return nil
	})
}

// pin records the portal job a batch confirmed as its archive. The job is
// verified, so the record always lands somewhere:
//
//   - the batch's own live claim (an abandoned IN_FLIGHT one is recovered
//     first) becomes CREATED with the job pinned, and must be exactly one
//     row;
//   - when another batch already holds the archive with a pinned job, the
//     archive is confirmed held and nothing changes;
//   - otherwise the confirmed job outranks any unconfirmed claim: those are
//     superseded and a pinned claim is recorded for this batch.
//
// A claim another live run is sending right now is an error.
func (g *downloadGuard) pin(ctx context.Context, id downloadIdentity, batchKey, jobID string) error {
	return g.inTx(ctx, func(tx *sql.Tx) error {
		now := isoUTC(time.Now())
		live, err := g.liveClaims(ctx, tx, id, batchKey)
		if err != nil {
			return err
		}
		var own *downloadClaim
		var others []downloadClaim
		for i := range live {
			c := live[i]
			if c.State == dgInFlight && !g.ownerAlive(guardClaim{OwnerRun: c.OwnerRun}) {
				c.State = dgUnknown
			}
			switch {
			case c.State == dgInFlight:
				return errDownloadClaimed{claim: c}
			case c.BatchKey == batchKey:
				own = &c
			case c.State == dgCreated && c.JobID != "":
				return nil
			default:
				others = append(others, c)
			}
		}
		if own != nil {
			res, err := tx.ExecContext(ctx, `UPDATE download_claims SET state = ?, job_id = ?, updated_at = ? WHERE id = ? AND superseded = 0`,
				dgCreated, jobID, now, own.ID)
			if err != nil {
				return err
			}
			if n, err := res.RowsAffected(); err != nil || n != 1 {
				return fmt.Errorf("pinning job %s: the download guard's claim %d did not take the update (%d rows)", jobID, own.ID, n)
			}
			return nil
		}
		generation := 1
		for _, c := range others {
			if _, err := tx.ExecContext(ctx, `UPDATE download_claims SET superseded = 1, superseded_by = ?, updated_at = ? WHERE id = ?`, batchKey, now, c.ID); err != nil {
				return err
			}
			generation = max(generation, c.Generation+1)
		}
		if last, err := g.lastClaim(ctx, tx, id); err != nil {
			return err
		} else if last != nil {
			generation = max(generation, last.Generation+1)
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO download_claims
			(account_id, task_id, entity, sub_source, snapshot, format, include_deleted, include_legal_hold,
			 generation, state, batch_key, job_id, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			id.Account, id.Task, id.Entity, id.SubSource, id.Snapshot, id.Format, id.IncludeDeleted, id.IncludeLegalHold,
			generation, dgCreated, batchKey, jobID, now, now)
		return err
	})
}

// hold records that a batch's archive cannot be told apart from another
// export: it may exist. The batch's own unconfirmed claim (an abandoned
// IN_FLIGHT one included) becomes UNKNOWN and must be exactly one row; a
// confirmed one is left alone; when another batch holds the archive that
// already blocks a second create; with no claim at all an UNKNOWN one is
// recorded for this batch.
func (g *downloadGuard) hold(ctx context.Context, id downloadIdentity, batchKey string) error {
	return g.inTx(ctx, func(tx *sql.Tx) error {
		now := isoUTC(time.Now())
		live, err := g.liveClaims(ctx, tx, id, batchKey)
		if err != nil {
			return err
		}
		var own *downloadClaim
		heldElsewhere := false
		for i := range live {
			c := live[i]
			if c.State == dgInFlight && !g.ownerAlive(guardClaim{OwnerRun: c.OwnerRun}) {
				c.State = dgUnknown
			}
			if c.BatchKey == batchKey {
				own = &c
			} else {
				heldElsewhere = true
			}
		}
		switch {
		case own != nil && own.State == dgCreated && own.JobID != "":
			return nil
		case own != nil && own.State == dgInFlight:
			return errDownloadClaimed{claim: *own}
		case own != nil:
			res, err := tx.ExecContext(ctx, `UPDATE download_claims SET state = ?, updated_at = ? WHERE id = ? AND superseded = 0`, dgUnknown, now, own.ID)
			if err != nil {
				return err
			}
			if n, err := res.RowsAffected(); err != nil || n != 1 {
				return fmt.Errorf("holding claim %d: the download guard did not take the update (%d rows)", own.ID, n)
			}
			return nil
		case heldElsewhere:
			return nil
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO download_claims
			(account_id, task_id, entity, sub_source, snapshot, format, include_deleted, include_legal_hold,
			 generation, state, batch_key, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?, ?, ?)`,
			id.Account, id.Task, id.Entity, id.SubSource, id.Snapshot, id.Format, id.IncludeDeleted, id.IncludeLegalHold,
			dgUnknown, batchKey, now, now)
		return err
	})
}

// jobFailed marks the claim pinned to jobID JOB_FAILED and returns it, or
// nil when no claim is pinned to that job.
func (g *downloadGuard) jobFailed(ctx context.Context, jobID string) (*downloadClaim, error) {
	var out *downloadClaim
	err := g.inTx(ctx, func(tx *sql.Tx) error {
		c, err := scanDownloadClaim(tx.QueryRowContext(ctx, `SELECT `+downloadClaimColumns+` FROM download_claims
			WHERE job_id = ? AND superseded = 0 AND state IN ('CREATED', 'JOB_FAILED') ORDER BY id DESC LIMIT 1`, jobID))
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		if c.State == dgCreated {
			res, err := tx.ExecContext(ctx, `UPDATE download_claims SET state = ?, updated_at = ? WHERE id = ?`, dgJobFailed, isoUTC(time.Now()), c.ID)
			if err != nil {
				return err
			}
			if n, err := res.RowsAffected(); err != nil || n != 1 {
				return fmt.Errorf("marking job %s failed: the download guard did not take the update (%d rows)", jobID, n)
			}
			c.State = dgJobFailed
		}
		out = &c
		return nil
	})
	return out, err
}

// jobReplaced reports whether the claim pinned to jobID has been
// superseded, or failed and been created again since: the job is history
// and nothing remains to be done about it.
func (g *downloadGuard) jobReplaced(ctx context.Context, jobID string) (bool, error) {
	var id, superseded int64
	var c downloadClaim
	err := g.db.QueryRowContext(ctx, `SELECT id, superseded, account_id, task_id, entity, sub_source, snapshot, format, include_deleted, include_legal_hold, state
		FROM download_claims WHERE job_id = ? ORDER BY id DESC LIMIT 1`, jobID).Scan(&id, &superseded,
		&c.Account, &c.Task, &c.Entity, &c.SubSource, &c.Snapshot, &c.Format, &c.IncludeDeleted, &c.IncludeLegalHold, &c.State)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if superseded == 1 {
		return true, nil
	}
	if c.State != dgJobFailed {
		return false, nil
	}
	var later int
	err = g.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM download_claims
		WHERE account_id = ? AND task_id = ? AND entity = ? AND sub_source = ? AND snapshot = ? AND format = ?
		AND include_deleted = ? AND include_legal_hold = ? AND id > ? AND superseded = 0 AND state IN ('IN_FLIGHT', 'UNKNOWN', 'CREATED')`,
		c.Account, c.Task, c.Entity, c.SubSource, c.Snapshot, c.Format, c.IncludeDeleted, c.IncludeLegalHold, id).Scan(&later)
	return later > 0, err
}

// replacement returns the claim that now holds the archive jobID was made
// for, when the guard proves one: a live claim of exactly that identity
// (format and both include flags), newer than jobID's own claim. Its JobID
// is empty while that create is not confirmed. nil when there is none, or
// when jobID's own claim is still the archive's current one.
func (g *downloadGuard) replacement(ctx context.Context, jobID string) (*downloadClaim, error) {
	var id int64
	var c downloadClaim
	err := g.db.QueryRowContext(ctx, `SELECT id, account_id, task_id, entity, sub_source, snapshot, format, include_deleted, include_legal_hold
		FROM download_claims WHERE job_id = ? ORDER BY id DESC LIMIT 1`, jobID).Scan(&id,
		&c.Account, &c.Task, &c.Entity, &c.SubSource, &c.Snapshot, &c.Format, &c.IncludeDeleted, &c.IncludeLegalHold)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	row := g.db.QueryRowContext(ctx, `SELECT `+downloadClaimColumns+` FROM download_claims
		WHERE account_id = ? AND task_id = ? AND entity = ? AND sub_source = ? AND snapshot = ? AND format = ?
		AND include_deleted = ? AND include_legal_hold = ? AND id > ? AND superseded = 0 AND state IN ('IN_FLIGHT', 'UNKNOWN', 'CREATED')
		ORDER BY id DESC LIMIT 1`,
		c.Account, c.Task, c.Entity, c.SubSource, c.Snapshot, c.Format, c.IncludeDeleted, c.IncludeLegalHold, id)
	cur, err := scanDownloadClaim(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if strings.EqualFold(strings.TrimSpace(cur.JobID), strings.TrimSpace(jobID)) {
		return nil, nil
	}
	return &cur, nil
}

// countActivity reads what, outside this run's own claims, could have moved
// the account's portal job count around since. unsettled: a create in
// flight or with an unknown outcome (it may land at any time, superseded
// or not); an accepted create not yet confirmed and not yet stale
// (staleRequestedAge), whose job may be counted late; or any claim made or
// changed from lag before since onwards, in any state, confirmed or not:
// another run can create and pin a job entirely between this run's two
// count reads. changed: any claim made or changed at or after since.
// Timestamps are whole seconds, so the second before a bound also counts:
// that only ever withholds EMPTY.
func (g *downloadGuard) countActivity(ctx context.Context, account string, since time.Time, lag time.Duration) (unsettled, changed bool, err error) {
	var u, c int
	early, open := isoUTC(since.Add(-lag)), isoUTC(since)
	fresh := isoUTC(since.Add(-staleRequestedAge))
	err = g.db.QueryRowContext(ctx, `SELECT
		COALESCE(SUM(state IN (?, ?) OR (state = ? AND job_id = '' AND updated_at >= ?) OR created_at >= ? OR updated_at >= ?), 0),
		COALESCE(SUM(created_at >= ? OR updated_at >= ?), 0)
		FROM download_claims WHERE account_id = ? AND owner_run <> ?`,
		dgInFlight, dgUnknown, dgCreated, fresh, early, early, open, open, account, g.run).Scan(&u, &c)
	return u > 0, c > 0, err
}

// narrowFlags records include flags an older ledger did not have on its
// seeded claims, once that batch's resume has stated them. Where a claim
// with exactly those flags is already live, the unrecorded one is
// superseded rather than duplicated.
func (g *downloadGuard) narrowFlags(ctx context.Context, b *downloadBatch) error {
	deleted, hold := flagValue(b.IncludeDeleted), flagValue(b.IncludeLegalHold)
	if deleted == flagNotRecorded || hold == flagNotRecorded {
		return nil
	}
	return g.inTx(ctx, func(tx *sql.Tx) error {
		now := isoUTC(time.Now())
		rows, err := tx.QueryContext(ctx, `SELECT `+downloadClaimColumns+` FROM download_claims
			WHERE batch_key = ? AND superseded = 0 AND (include_deleted = -1 OR include_legal_hold = -1)`, b.BatchKey)
		if err != nil {
			return err
		}
		var seeded []downloadClaim
		for rows.Next() {
			c, err := scanDownloadClaim(rows)
			if err != nil {
				rows.Close()
				return err
			}
			seeded = append(seeded, c)
		}
		rows.Close()
		for _, c := range seeded {
			exact := c.downloadIdentity
			if exact.IncludeDeleted == flagNotRecorded {
				exact.IncludeDeleted = deleted
			}
			if exact.IncludeLegalHold == flagNotRecorded {
				exact.IncludeLegalHold = hold
			}
			var clash int
			if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM download_claims
				WHERE account_id = ? AND task_id = ? AND entity = ? AND sub_source = ? AND snapshot = ? AND format = ?
				AND include_deleted = ? AND include_legal_hold = ? AND id <> ? AND superseded = 0 AND state IN ('IN_FLIGHT', 'UNKNOWN', 'CREATED')`,
				exact.Account, exact.Task, exact.Entity, exact.SubSource, exact.Snapshot, exact.Format,
				exact.IncludeDeleted, exact.IncludeLegalHold, c.ID).Scan(&clash); err != nil {
				return err
			}
			if clash > 0 {
				if _, err := tx.ExecContext(ctx, `UPDATE download_claims SET superseded = 1, superseded_by = ?, updated_at = ? WHERE id = ?`, b.BatchKey, now, c.ID); err != nil {
					return err
				}
				continue
			}
			if _, err := tx.ExecContext(ctx, `UPDATE download_claims SET include_deleted = ?, include_legal_hold = ?, updated_at = ? WHERE id = ?`,
				exact.IncludeDeleted, exact.IncludeLegalHold, now, c.ID); err != nil {
				return err
			}
		}
		return nil
	})
}

// labels returns every job label the guard knows, mapped to the claim's
// identity, so a listed job carrying one is known to be that claim's.
func (g *downloadGuard) labels(ctx context.Context) (map[string]downloadIdentity, error) {
	rows, err := g.db.QueryContext(ctx, `SELECT `+downloadClaimColumns+` FROM download_claims WHERE label <> ''`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]downloadIdentity{}
	for rows.Next() {
		c, err := scanDownloadClaim(rows)
		if err != nil {
			return nil, err
		}
		out[c.Label] = c.downloadIdentity
	}
	return out, rows.Err()
}

// seedEvidence is what one ledger row says about one archive.
type seedEvidence struct {
	id                     downloadIdentity
	batchKey, scope, label string
	jobID                  string
	rank                   int
	state                  string
}

// Evidence ranks: a confirmed job outranks an accepted create, which
// outranks a create whose outcome is unknown.
const (
	rankUnknown = 1
	rankCreated = 2
	rankPinned  = 3
)

// seedFromLedgers imports every archive the JSON batch ledgers record as
// created or possibly created, once per ledger file, so an archive made
// before the guard existed, or by a build without it, still blocks a
// second create. Ledgers are read, never changed, and one that cannot be
// read fails the run closed.
//
// Evidence about one archive from several ledgers is merged, never decided
// by which ledger was read first: everything not yet imported is gathered,
// applied strongest first, and merged with what the guard already holds
// (mergeEvidence). A confirmed job from any batch wins.
func (g *downloadGuard) seedFromLedgers(ctx context.Context) error {
	dir, err := batchDir()
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var paths []string
	var evidence []seedEvidence
	for _, e := range entries {
		key, ok := strings.CutSuffix(e.Name(), ".json")
		if !ok || validBatchKey(key) != nil {
			continue
		}
		path := filepath.Join(dir, e.Name())
		var seen int
		if err := g.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM seeded_ledgers WHERE path = ?`, path).Scan(&seen); err != nil {
			return fmt.Errorf("reading the download guard %s: %w", g.path, err)
		}
		if seen > 0 {
			continue
		}
		b, err := loadBatch(key)
		if err != nil {
			return fmt.Errorf("batch ledger %s cannot be read (%v), so the archives it records cannot be checked; nothing was sent. Fix or move that file, then run again", path, err)
		}
		paths = append(paths, path)
		for i := range b.Rows {
			r := &b.Rows[i]
			if !mayExist(*r) && !legacyMaybeSent(b, *r) {
				continue
			}
			ev := seedEvidence{id: rowIdentity(b, r), batchKey: b.BatchKey, scope: b.Scope, label: r.Label, rank: rankUnknown, state: dgUnknown}
			switch {
			case r.Status == dlCreated && r.JobID != "":
				ev.rank, ev.state, ev.jobID = rankPinned, dgCreated, r.JobID
			case r.Status == dlEmpty, r.Status == dlCreated, r.Status == dlMissing, r.AcceptedAt != "":
				ev.rank, ev.state = rankCreated, dgCreated
			}
			evidence = append(evidence, ev)
		}
	}
	if len(paths) == 0 {
		return nil
	}
	sort.SliceStable(evidence, func(i, j int) bool {
		if evidence[i].rank != evidence[j].rank {
			return evidence[i].rank > evidence[j].rank
		}
		return evidence[i].batchKey < evidence[j].batchKey
	})
	if err := g.inTx(ctx, func(tx *sql.Tx) error {
		now := isoUTC(time.Now())
		for _, ev := range evidence {
			if err := g.mergeEvidence(ctx, tx, ev, now); err != nil {
				return err
			}
		}
		for _, path := range paths {
			if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO seeded_ledgers (path, imported, seeded_at) VALUES (?, ?, ?)`, path, len(evidence), now); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return fmt.Errorf("seeding the download guard %s from the batch ledgers: %w; nothing was sent", g.path, err)
	}
	return nil
}

// mergeEvidence folds one ledger row's evidence into the live claims for
// its archive:
//
//   - no live claim: the evidence becomes one;
//   - a confirmed (pinned) claim holds: nothing changes;
//   - pinned evidence outranks unconfirmed claims: they are superseded and
//     the pinned claim recorded;
//   - unconfirmed evidence from another batch than the unconfirmed claim's:
//     either batch's create may exist, so the claim becomes UNKNOWN (no
//     batch creates it again on its own) under the lower batch key, which
//     does not depend on the order ledgers were read;
//   - unconfirmed evidence from the claim's own batch keeps the stronger
//     state.
func (g *downloadGuard) mergeEvidence(ctx context.Context, tx *sql.Tx, ev seedEvidence, now string) error {
	live, err := g.liveClaims(ctx, tx, ev.id, ev.batchKey)
	if err != nil {
		return err
	}
	var unconfirmed []downloadClaim
	for _, c := range live {
		if c.State == dgInFlight && !g.ownerAlive(guardClaim{OwnerRun: c.OwnerRun}) {
			c.State = dgUnknown
			if _, err := tx.ExecContext(ctx, `UPDATE download_claims SET state = ?, updated_at = ? WHERE id = ?`, dgUnknown, now, c.ID); err != nil {
				return err
			}
		}
		switch {
		case c.State == dgInFlight:
			return nil
		case c.State == dgCreated && c.JobID != "":
			return nil
		default:
			unconfirmed = append(unconfirmed, c)
		}
	}
	insert := func(generation int) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO download_claims
			(account_id, task_id, entity, sub_source, snapshot, format, include_deleted, include_legal_hold,
			 generation, state, batch_key, job_id, label, scope, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			ev.id.Account, ev.id.Task, ev.id.Entity, ev.id.SubSource, ev.id.Snapshot, ev.id.Format, ev.id.IncludeDeleted, ev.id.IncludeLegalHold,
			generation, ev.state, ev.batchKey, ev.jobID, ev.label, ev.scope, now, now)
		return err
	}
	if len(unconfirmed) == 0 {
		return insert(1)
	}
	if ev.rank == rankPinned {
		generation := 1
		for _, c := range unconfirmed {
			if _, err := tx.ExecContext(ctx, `UPDATE download_claims SET superseded = 1, superseded_by = ?, updated_at = ? WHERE id = ?`, ev.batchKey, now, c.ID); err != nil {
				return err
			}
			generation = max(generation, c.Generation+1)
		}
		return insert(generation)
	}
	for _, c := range unconfirmed {
		switch {
		case c.BatchKey != ev.batchKey:
			owner := min(c.BatchKey, ev.batchKey)
			if _, err := tx.ExecContext(ctx, `UPDATE download_claims SET state = ?, batch_key = ?, updated_at = ? WHERE id = ?`, dgUnknown, owner, now, c.ID); err != nil {
				return err
			}
		case ev.state == dgCreated && c.State == dgUnknown:
			if _, err := tx.ExecContext(ctx, `UPDATE download_claims SET state = ?, updated_at = ? WHERE id = ?`, dgCreated, now, c.ID); err != nil {
				return err
			}
		}
	}
	return nil
}
