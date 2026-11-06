// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): the last state `issues watch` saw for each
// issue, so the next poll can tell what changed. Pinned by this connector's
// handfixes.json under the "issues-watch-events" entry. Created from
// migrateExtras (extras.go). data holds the minimal issue record the watcher
// keeps (ids and triage fields), never a description or comment body.

package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"servosity-msp-pp-cli/internal/credkit"
)

var issueWatchMigrations = []string{
	`CREATE TABLE IF NOT EXISTS issue_watch_state (
		issue_id TEXT PRIMARY KEY,
		data TEXT NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS issue_watch_generation (
		id INTEGER PRIMARY KEY CHECK (id = 1),
		gen INTEGER NOT NULL
	)`,
}

// IssueWatchSnapshot is what a poll bases its writes on: the recorded issue
// records by issue id, and the generation of the table when they were read.
type IssueWatchSnapshot struct {
	Records map[string]string
	Gen     int64
}

// IssueWatchState reads the snapshot a poll bases every write on (see
// ApplyIssueWatch). The generation is read first: a commit landing between
// the two reads leaves an older generation with newer records, which only
// makes the poll stale, never lets an older read apply. A store that has
// never run the watcher has no tables yet: that reads as empty.
func (s *Store) IssueWatchState(ctx context.Context) (IssueWatchSnapshot, error) {
	snap := IssueWatchSnapshot{Records: map[string]string{}}
	if err := s.db.QueryRowContext(ctx, `SELECT gen FROM issue_watch_generation WHERE id = 1`).Scan(&snap.Gen); err != nil &&
		err != sql.ErrNoRows && !strings.Contains(err.Error(), "no such table") {
		return snap, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT issue_id, data FROM issue_watch_state`)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return snap, nil
		}
		return snap, err
	}
	defer rows.Close()
	for rows.Next() {
		var id, data string
		if err := rows.Scan(&id, &data); err != nil {
			return snap, err
		}
		snap.Records[id] = data
	}
	return snap, rows.Err()
}

// IssueWrite is one compare-and-swap on a recorded issue. Expect is the
// record the caller read before the API ("" when the issue was not
// recorded); Data is the new record, or "" to forget the issue. It applies,
// with its Events, only while the stored record is still exactly Expect.
type IssueWrite struct {
	ID, Expect, Data string
	Events           []VendorEvent
}

// casIssueWrite applies w if the stored record is still w.Expect, in SQL,
// and reports whether it did.
func casIssueWrite(ctx context.Context, tx *sql.Tx, w IssueWrite) (bool, error) {
	var res sql.Result
	var err error
	switch {
	case w.Data == "":
		res, err = tx.ExecContext(ctx, `DELETE FROM issue_watch_state WHERE issue_id = ? AND data = ?`, w.ID, w.Expect)
	case w.Expect == "":
		res, err = tx.ExecContext(ctx, `INSERT INTO issue_watch_state (issue_id, data) VALUES (?, ?)
			ON CONFLICT(issue_id) DO NOTHING`, w.ID, w.Data)
	default:
		res, err = tx.ExecContext(ctx, `UPDATE issue_watch_state SET data = ? WHERE issue_id = ? AND data = ?`, w.Data, w.ID, w.Expect)
	}
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}

// IssueWatchApplied is what ApplyIssueWatch did.
type IssueWatchApplied struct {
	Added, Stale, Withheld int
	Cursor                 int64
}

// ApplyIssueWatch records one observation in a single write transaction.
// decide receives whether mark has a watermark (read inside the
// transaction) and returns the writes; an error from decide writes nothing.
// gen is the generation of the snapshot the caller based the writes on
// (IssueWatchState). Every commit bumps the generation, so when another
// watcher committed anything since that read, nothing applies: every write
// counts as stale and the next poll decides. The generation only grows, so a
// record that changed and changed back (absent, present, absent again) still
// reads as changed. Each write is also a compare-and-swap (IssueWrite). A
// duplicate event_id or delivery_id is skipped. When setMark is true the
// watermark for mark is set to at. A write the credential kit's SQL driver
// withholds (it repeats a held credential) rolls back alone, with its events,
// and the rest still apply. It returns how many events were new, how many
// writes were stale, how many were withheld, and the latest cursor. ponytail: one generation
// for the whole table, so two watchers polling at once lose the slower
// poll's writes to the next poll; per-issue versions if they ever contend.
func (s *Store) ApplyIssueWatch(ctx context.Context, mark string, setMark bool, at string, gen int64,
	decide func(watched bool) ([]IssueWrite, error)) (IssueWatchApplied, error) {
	s.lockForWrite()
	defer s.unlockAfterWrite()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return IssueWatchApplied{}, err
	}
	defer tx.Rollback()
	var one int
	watched := tx.QueryRowContext(ctx, `SELECT 1 FROM vendor_event_watermarks WHERE source = ?`, mark).Scan(&one) == nil
	writes, err := decide(watched)
	if err != nil {
		return IssueWatchApplied{}, err
	}
	var current int64
	if err := tx.QueryRowContext(ctx, `SELECT gen FROM issue_watch_generation WHERE id = 1`).Scan(&current); err != nil && err != sql.ErrNoRows {
		return IssueWatchApplied{}, err
	}
	var cursor int64
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(seq), 0) FROM vendor_events`).Scan(&cursor); err != nil {
		return IssueWatchApplied{}, err
	}
	if current != gen {
		return IssueWatchApplied{Stale: len(writes), Cursor: cursor}, nil
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO issue_watch_generation (id, gen) VALUES (1, ?)
		ON CONFLICT(id) DO UPDATE SET gen = excluded.gen`, gen+1); err != nil {
		return IssueWatchApplied{}, err
	}
	var out IssueWatchApplied
	for _, w := range writes {
		added, ok, err := applyIssueWrite(ctx, tx, w)
		switch {
		case errors.Is(err, credkit.ErrWithheld):
			out.Withheld++
		case err != nil:
			return IssueWatchApplied{}, err
		case !ok:
			out.Stale++
		default:
			out.Added += added
		}
	}
	if setMark {
		if _, err := tx.ExecContext(ctx, `INSERT INTO vendor_event_watermarks (source, since) VALUES (?, ?)
			ON CONFLICT(source) DO UPDATE SET since = excluded.since`, mark, at); err != nil {
			return IssueWatchApplied{}, err
		}
	}
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(seq), 0) FROM vendor_events`).Scan(&cursor); err != nil {
		return IssueWatchApplied{}, err
	}
	out.Cursor = cursor
	return out, tx.Commit()
}

// applyIssueWrite applies one write and its events under a savepoint, so a
// statement the credential kit withholds leaves nothing of that write behind.
// It reports how many events were new and whether the compare-and-swap held.
func applyIssueWrite(ctx context.Context, tx *sql.Tx, w IssueWrite) (int, bool, error) {
	if _, err := tx.ExecContext(ctx, `SAVEPOINT issue_write`); err != nil {
		return 0, false, err
	}
	added, ok, err := execIssueWrite(ctx, tx, w)
	if err != nil && !errors.Is(err, credkit.ErrWithheld) {
		return 0, false, err // the caller rolls the whole transaction back
	}
	if err != nil {
		if _, rerr := tx.ExecContext(ctx, `ROLLBACK TO issue_write`); rerr != nil {
			return 0, false, rerr
		}
	}
	if _, rerr := tx.ExecContext(ctx, `RELEASE issue_write`); rerr != nil {
		return 0, false, rerr
	}
	return added, ok, err
}

func execIssueWrite(ctx context.Context, tx *sql.Tx, w IssueWrite) (int, bool, error) {
	ok, err := casIssueWrite(ctx, tx, w)
	if err != nil || !ok {
		return 0, false, err
	}
	added := 0
	for _, e := range w.Events {
		var delivery any
		if e.DeliveryID != "" {
			delivery = e.DeliveryID
		}
		data := string(e.Data)
		if data == "" {
			data = "{}"
		}
		res, err := tx.ExecContext(ctx, insertVendorEventSQL,
			e.EventID, e.Type, e.Source, e.OccurredAt, e.ReceivedAt, e.ResourceType, e.ResourceID, delivery, e.Verified, data)
		if err != nil {
			return 0, false, err
		}
		if n, _ := res.RowsAffected(); n > 0 {
			added++
		}
	}
	return added, true, nil
}
