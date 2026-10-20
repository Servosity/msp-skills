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
//   - JOB_FAILED: the confirmed job finished failed or cancelled. Not live:
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
	return &downloadGuard{g}, nil
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
func (g *downloadGuard) claim(ctx context.Context, id downloadIdentity, owner downloadOwner, retryUnknown, newBatch bool) (int64, error) {
	var claimID int64
	err := g.inTx(ctx, func(tx *sql.Tx) error {
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
			case newBatch:
				if _, err := tx.ExecContext(ctx, `UPDATE download_claims SET superseded = 1, superseded_by = ?, updated_at = ? WHERE id = ?`, owner.batchKey, now, c.ID); err != nil {
					return err
				}
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
			claimID = reuse.ID
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
	return claimID, err
}

// settle records what the create for a claim returned: accepted is
// CREATED, refused before it ran is REJECTED (which frees the identity),
// anything else stays UNKNOWN.
func (g *downloadGuard) settle(ctx context.Context, claimID int64, rowStatus string, accepted bool) error {
	state := dgUnknown
	switch {
	case rowStatus == dlFailed:
		state = dgRejected
	case accepted:
		state = dgCreated
	}
	return g.inTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE download_claims SET state = ?, updated_at = ? WHERE id = ? AND owner_run = ?`,
			state, isoUTC(time.Now()), claimID, g.run)
		return err
	})
}

// pin records the portal job a batch confirmed as its archive.
func (g *downloadGuard) pin(ctx context.Context, id downloadIdentity, batchKey, jobID string) error {
	return g.inTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE download_claims SET state = ?, job_id = ?, updated_at = ?
			WHERE `+identityWhere+` AND batch_key = ? AND superseded = 0 AND state IN ('UNKNOWN', 'CREATED')`,
			append([]any{dgCreated, jobID, isoUTC(time.Now())}, append(id.args(), batchKey)...)...)
		return err
	})
}

// hold marks a batch's unconfirmed claim UNKNOWN: its job cannot be told
// apart from another export. A pinned claim is left as it is.
func (g *downloadGuard) hold(ctx context.Context, id downloadIdentity, batchKey string) error {
	return g.inTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE download_claims SET state = ?, updated_at = ?
			WHERE `+identityWhere+` AND batch_key = ? AND superseded = 0 AND state = 'CREATED' AND job_id = ''`,
			append([]any{dgUnknown, isoUTC(time.Now())}, append(id.args(), batchKey)...)...)
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
			if _, err := tx.ExecContext(ctx, `UPDATE download_claims SET state = ?, updated_at = ? WHERE id = ?`, dgJobFailed, isoUTC(time.Now()), c.ID); err != nil {
				return err
			}
			c.State = dgJobFailed
		}
		out = &c
		return nil
	})
	return out, err
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

// seedFromLedgers imports, once per ledger file, every archive any JSON
// batch ledger records as created or possibly created, so an archive made
// before the guard existed, or by a build without it, still blocks a second
// create. Ledgers are read, never changed. A ledger that cannot be read
// fails the run closed.
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
		if err := g.inTx(ctx, func(tx *sql.Tx) error {
			if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM seeded_ledgers WHERE path = ?`, path).Scan(&seen); err != nil || seen > 0 {
				return err
			}
			now := isoUTC(time.Now())
			imported := 0
			for i := range b.Rows {
				r := &b.Rows[i]
				if !mayExist(*r) {
					continue
				}
				id := rowIdentity(b, r)
				live, err := g.liveClaims(ctx, tx, id, b.BatchKey)
				if err != nil {
					return err
				}
				if len(live) > 0 {
					continue
				}
				state, jobID := dgUnknown, ""
				switch {
				case r.Status == dlCreated && r.JobID != "":
					state, jobID = dgCreated, r.JobID
				case r.Status == dlEmpty, r.Status == dlCreated, r.Status == dlMissing, r.AcceptedAt != "":
					state = dgCreated
				}
				if _, err := tx.ExecContext(ctx, `INSERT INTO download_claims
					(account_id, task_id, entity, sub_source, snapshot, format, include_deleted, include_legal_hold,
					 generation, state, batch_key, job_id, label, scope, created_at, updated_at)
					VALUES (?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?, ?, ?, ?, ?, ?)`,
					id.Account, id.Task, id.Entity, id.SubSource, id.Snapshot, id.Format, id.IncludeDeleted, id.IncludeLegalHold,
					state, b.BatchKey, jobID, r.Label, b.Scope, now, now); err != nil {
					return err
				}
				imported++
			}
			_, err := tx.ExecContext(ctx, `INSERT INTO seeded_ledgers (path, imported, seeded_at) VALUES (?, ?, ?)`, path, imported, now)
			return err
		}); err != nil {
			return fmt.Errorf("seeding the download guard %s from %s: %w; nothing was sent", g.path, path, err)
		}
	}
	return nil
}
