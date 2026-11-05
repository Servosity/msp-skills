// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// keeper handfix posture-store: the tables 'posture run' fills and
// 'posture diff', 'posture history' and 'events list' read. Created from
// migrateExtras (extras.go).
//
// vendor_events follows the zammad connector's shape so it is ready for MCP
// Events (draft) without implementing it: seq is the cursor, event_id the
// stable id (UNIQUE, so a re-run never repeats an event), type the event
// name, data the minimal before/after payload.

package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"
)

var keeperMigrations = []string{
	`CREATE TABLE IF NOT EXISTS keeper_posture_runs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		started_at TEXT NOT NULL,
		finished_at TEXT NOT NULL DEFAULT '',
		inactive_days INTEGER NOT NULL,
		status TEXT NOT NULL DEFAULT 'running'
	)`,
	// One row per company per run. ok=0 rows carry the read error and no
	// metrics; diffs skip them (a failed read is never "everything fixed").
	`CREATE TABLE IF NOT EXISTS keeper_company_posture (
		run_id INTEGER NOT NULL,
		company_id TEXT NOT NULL,
		company_name TEXT NOT NULL DEFAULT '',
		ok INTEGER NOT NULL,
		error TEXT NOT NULL DEFAULT '',
		metrics TEXT NOT NULL DEFAULT '{}',
		PRIMARY KEY (run_id, company_id)
	)`,
	`CREATE TABLE IF NOT EXISTS keeper_user_posture (
		run_id INTEGER NOT NULL,
		company_id TEXT NOT NULL,
		email TEXT NOT NULL,
		facts TEXT NOT NULL DEFAULT '{}',
		PRIMARY KEY (run_id, company_id, email)
	)`,
	`CREATE TABLE IF NOT EXISTS vendor_events (
		seq INTEGER PRIMARY KEY AUTOINCREMENT,
		event_id TEXT NOT NULL UNIQUE,
		type TEXT NOT NULL,
		source TEXT NOT NULL CHECK (source IN ('sweep', 'webhook', 'sync', 'poll')),
		occurred_at TEXT NOT NULL,
		received_at TEXT NOT NULL,
		resource_type TEXT NOT NULL,
		resource_id TEXT NOT NULL,
		client TEXT NOT NULL DEFAULT '',
		data TEXT NOT NULL DEFAULT '{}'
	)`,
	`CREATE INDEX IF NOT EXISTS idx_vendor_events_type_seq ON vendor_events(type, seq)`,
}

func keeperNow() string { return time.Now().UTC().Format(time.RFC3339Nano) }

func keeperMissingTable(err error) bool {
	return err != nil && strings.Contains(err.Error(), "no such table")
}

// CompanyPosture is one company's result in one run.
type CompanyPosture struct {
	RunID       int64           `json:"run_id"`
	CompanyID   string          `json:"company_id"`
	CompanyName string          `json:"company_name"`
	OK          bool            `json:"ok"`
	Error       string          `json:"error,omitempty"`
	Metrics     json.RawMessage `json:"metrics"`
}

// UserPosture is one user's facts in one company in one run.
type UserPosture struct {
	CompanyID string          `json:"company_id"`
	Email     string          `json:"email"`
	Facts     json.RawMessage `json:"facts"`
}

// PostureRun is one keeper_posture_runs row.
type PostureRun struct {
	ID           int64  `json:"id"`
	StartedAt    string `json:"started_at"`
	FinishedAt   string `json:"finished_at"`
	InactiveDays int    `json:"inactive_days"`
	Status       string `json:"status"`
}

// BeginPostureRun records a new run and returns its id.
func (s *Store) BeginPostureRun(ctx context.Context, inactiveDays int) (int64, error) {
	s.lockForWrite()
	defer s.unlockAfterWrite()
	res, err := s.db.ExecContext(ctx, `INSERT INTO keeper_posture_runs (started_at, inactive_days) VALUES (?, ?)`, keeperNow(), inactiveDays)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// FinishPostureRun stamps a run's end and status (complete, partial, failed).
func (s *Store) FinishPostureRun(ctx context.Context, runID int64, status string) error {
	s.lockForWrite()
	defer s.unlockAfterWrite()
	_, err := s.db.ExecContext(ctx, `UPDATE keeper_posture_runs SET finished_at = ?, status = ? WHERE id = ?`, keeperNow(), status, runID)
	return err
}

// PutCompanyPosture writes one company's result and its users in one transaction.
func (s *Store) PutCompanyPosture(ctx context.Context, p CompanyPosture, users []UserPosture) error {
	s.lockForWrite()
	defer s.unlockAfterWrite()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	metrics := string(p.Metrics)
	if metrics == "" {
		metrics = "{}"
	}
	ok := 0
	if p.OK {
		ok = 1
	}
	if _, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO keeper_company_posture (run_id, company_id, company_name, ok, error, metrics) VALUES (?, ?, ?, ?, ?, ?)`,
		p.RunID, p.CompanyID, p.CompanyName, ok, p.Error, metrics); err != nil {
		return err
	}
	for _, u := range users {
		facts := string(u.Facts)
		if facts == "" {
			facts = "{}"
		}
		if _, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO keeper_user_posture (run_id, company_id, email, facts) VALUES (?, ?, ?, ?)`,
			p.RunID, p.CompanyID, u.Email, facts); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// PostureRuns returns the newest runs first.
func (s *Store) PostureRuns(ctx context.Context, limit int) ([]PostureRun, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, started_at, finished_at, inactive_days, status FROM keeper_posture_runs ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		if keeperMissingTable(err) {
			return []PostureRun{}, nil
		}
		return nil, err
	}
	defer rows.Close()
	out := []PostureRun{}
	for rows.Next() {
		var r PostureRun
		if err := rows.Scan(&r.ID, &r.StartedAt, &r.FinishedAt, &r.InactiveDays, &r.Status); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// CompanyPostures returns every company row of one run, by company id.
func (s *Store) CompanyPostures(ctx context.Context, runID int64) ([]CompanyPosture, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT run_id, company_id, company_name, ok, error, metrics FROM keeper_company_posture WHERE run_id = ? ORDER BY company_id`, runID)
	if err != nil {
		if keeperMissingTable(err) {
			return []CompanyPosture{}, nil
		}
		return nil, err
	}
	defer rows.Close()
	out := []CompanyPosture{}
	for rows.Next() {
		var p CompanyPosture
		var ok int
		var metrics string
		if err := rows.Scan(&p.RunID, &p.CompanyID, &p.CompanyName, &ok, &p.Error, &metrics); err != nil {
			return nil, err
		}
		p.OK = ok == 1
		p.Metrics = json.RawMessage(metrics)
		out = append(out, p)
	}
	return out, rows.Err()
}

// PreviousCompanyError is the error of the company's newest row before beforeRun,
// or "" when that row was good or there is none.
func (s *Store) PreviousCompanyError(ctx context.Context, companyID string, beforeRun int64) (string, error) {
	var msg sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT error FROM keeper_company_posture
		WHERE company_id = ? AND run_id < ? ORDER BY run_id DESC LIMIT 1`, companyID, beforeRun).Scan(&msg)
	if err == sql.ErrNoRows || keeperMissingTable(err) {
		return "", nil
	}
	return msg.String, err
}

// LastGoodCompanyPosture returns a company's newest ok=1 row from a run
// before beforeRun, or false when there is none.
func (s *Store) LastGoodCompanyPosture(ctx context.Context, companyID string, beforeRun int64) (CompanyPosture, bool, error) {
	var p CompanyPosture
	var metrics string
	err := s.db.QueryRowContext(ctx, `SELECT run_id, company_id, company_name, metrics FROM keeper_company_posture
		WHERE company_id = ? AND ok = 1 AND run_id < ? ORDER BY run_id DESC LIMIT 1`, companyID, beforeRun).
		Scan(&p.RunID, &p.CompanyID, &p.CompanyName, &metrics)
	if err == sql.ErrNoRows || keeperMissingTable(err) {
		return CompanyPosture{}, false, nil
	}
	if err != nil {
		return CompanyPosture{}, false, err
	}
	p.OK = true
	p.Metrics = json.RawMessage(metrics)
	return p, true, nil
}

// UserPostures returns one company's users in one run, by email.
func (s *Store) UserPostures(ctx context.Context, runID int64, companyID string) ([]UserPosture, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT company_id, email, facts FROM keeper_user_posture WHERE run_id = ? AND company_id = ? ORDER BY email`, runID, companyID)
	if err != nil {
		if keeperMissingTable(err) {
			return []UserPosture{}, nil
		}
		return nil, err
	}
	defer rows.Close()
	out := []UserPosture{}
	for rows.Next() {
		var u UserPosture
		var facts string
		if err := rows.Scan(&u.CompanyID, &u.Email, &facts); err != nil {
			return nil, err
		}
		u.Facts = json.RawMessage(facts)
		out = append(out, u)
	}
	return out, rows.Err()
}

// VendorEvent is one vendor_events row.
type VendorEvent struct {
	Seq          int64           `json:"seq"`
	EventID      string          `json:"event_id"`
	Type         string          `json:"type"`
	Source       string          `json:"source"`
	OccurredAt   string          `json:"occurred_at"`
	ReceivedAt   string          `json:"received_at"`
	ResourceType string          `json:"resource_type"`
	ResourceID   string          `json:"resource_id"`
	Client       string          `json:"client,omitempty"`
	Data         json.RawMessage `json:"data"`
}

// InsertVendorEvent writes e unless its event_id is already recorded. It
// returns the new seq, or 0 for a duplicate.
func (s *Store) InsertVendorEvent(ctx context.Context, e VendorEvent) (int64, error) {
	data := string(e.Data)
	if data == "" {
		data = "{}"
	}
	if e.ReceivedAt == "" {
		e.ReceivedAt = keeperNow()
	}
	s.lockForWrite()
	defer s.unlockAfterWrite()
	res, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO vendor_events
		(event_id, type, source, occurred_at, received_at, resource_type, resource_id, client, data)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.EventID, e.Type, e.Source, e.OccurredAt, e.ReceivedAt, e.ResourceType, e.ResourceID, e.Client, data)
	if err != nil {
		return 0, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return 0, nil
	}
	return res.LastInsertId()
}

// VendorEventsSince returns up to limit rows after cursor seq, oldest first,
// optionally of one type, and whether more remain. A store that never ran a
// posture run has no table yet: that reads as no events.
func (s *Store) VendorEventsSince(ctx context.Context, since int64, eventType string, limit int) ([]VendorEvent, bool, error) {
	q := `SELECT seq, event_id, type, source, occurred_at, received_at, resource_type, resource_id, client, data
		FROM vendor_events WHERE seq > ?`
	args := []any{since}
	if eventType != "" {
		q += ` AND type = ?`
		args = append(args, eventType)
	}
	q += ` ORDER BY seq LIMIT ?`
	args = append(args, limit+1)
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		if keeperMissingTable(err) {
			return []VendorEvent{}, false, nil
		}
		return nil, false, err
	}
	defer rows.Close()
	out := []VendorEvent{}
	for rows.Next() {
		var e VendorEvent
		var data string
		if err := rows.Scan(&e.Seq, &e.EventID, &e.Type, &e.Source, &e.OccurredAt, &e.ReceivedAt, &e.ResourceType, &e.ResourceID, &e.Client, &data); err != nil {
			return nil, false, err
		}
		e.Data = json.RawMessage(data)
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	if len(out) > limit {
		return out[:limit], true, nil
	}
	return out, false, nil
}

// MaxEventSeq returns the newest event cursor (0 when there is no event or
// no event table yet).
func (s *Store) MaxEventSeq(ctx context.Context) (int64, error) {
	var seq sql.NullInt64
	if err := s.db.QueryRowContext(ctx, `SELECT MAX(seq) FROM vendor_events`).Scan(&seq); err != nil {
		if keeperMissingTable(err) {
			return 0, nil
		}
		return 0, err
	}
	return seq.Int64, nil
}

// LatestGoodPerCompany returns each company's newest ok=1 row with the time
// of the run that took it, by company id.
func (s *Store) LatestGoodPerCompany(ctx context.Context) ([]CompanyPosture, map[int64]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT p.run_id, p.company_id, p.company_name, p.metrics, r.started_at
		FROM keeper_company_posture p JOIN keeper_posture_runs r ON r.id = p.run_id
		WHERE p.ok = 1 AND p.run_id = (SELECT MAX(q.run_id) FROM keeper_company_posture q WHERE q.company_id = p.company_id AND q.ok = 1)
		ORDER BY p.company_id`)
	if err != nil {
		if keeperMissingTable(err) {
			return []CompanyPosture{}, map[int64]string{}, nil
		}
		return nil, nil, err
	}
	defer rows.Close()
	out := []CompanyPosture{}
	taken := map[int64]string{}
	for rows.Next() {
		var p CompanyPosture
		var metrics, at string
		if err := rows.Scan(&p.RunID, &p.CompanyID, &p.CompanyName, &metrics, &at); err != nil {
			return nil, nil, err
		}
		p.OK = true
		p.Metrics = json.RawMessage(metrics)
		taken[p.RunID] = at
		out = append(out, p)
	}
	return out, taken, rows.Err()
}

// FirstSeen returns, for each user (lowercased email) in a company's newest
// good run, the start time of the first run of the user's current state: the
// earliest good run of the unbroken series of good runs, ending with the
// newest, in which the same user (same enterprise user id) had the same
// status (keeper handfix invite-age-by-identity, Codex r1 class
// wrong-invite-age). A good run where the user is missing, has another user
// id (deleted and re-invited) or another status ends the series; a failed
// run (no user rows) is skipped. A run stored without user ids ends it too,
// so the age is never overstated.
func (s *Store) FirstSeen(ctx context.Context, companyID string) (map[string]string, error) {
	type state struct{ UserID, Status string }
	runs, err := s.db.QueryContext(ctx, `SELECT p.run_id, r.started_at FROM keeper_company_posture p
		JOIN keeper_posture_runs r ON r.id = p.run_id WHERE p.company_id = ? AND p.ok = 1 ORDER BY p.run_id DESC`, companyID)
	if err != nil {
		if keeperMissingTable(err) {
			return map[string]string{}, nil
		}
		return nil, err
	}
	var order []int64
	started := map[int64]string{}
	for runs.Next() {
		var id int64
		var at string
		if err := runs.Scan(&id, &at); err != nil {
			_ = runs.Close() // the Scan error is the one returned
			return nil, err
		}
		order = append(order, id)
		started[id] = at
	}
	if err := runs.Close(); err != nil {
		return nil, err
	}
	if err := runs.Err(); err != nil {
		return nil, err
	}
	out := map[string]string{}
	if len(order) == 0 {
		return out, nil
	}
	rows, err := s.db.QueryContext(ctx, `SELECT u.run_id, u.email, u.facts FROM keeper_user_posture u
		JOIN keeper_company_posture p ON p.run_id = u.run_id AND p.company_id = u.company_id
		WHERE u.company_id = ? AND p.ok = 1`, companyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	byRun := map[int64]map[string]state{}
	for rows.Next() {
		var run int64
		var email, facts string
		if err := rows.Scan(&run, &email, &facts); err != nil {
			return nil, err
		}
		var st state
		var raw struct {
			UserID string `json:"user_id"`
			Status string `json:"status"`
		}
		if json.Unmarshal([]byte(facts), &raw) == nil {
			st = state{UserID: raw.UserID, Status: raw.Status}
		}
		if byRun[run] == nil {
			byRun[run] = map[string]state{}
		}
		byRun[run][email] = st
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for email, cur := range byRun[order[0]] {
		since := started[order[0]]
		for _, run := range order[1:] {
			prev, ok := byRun[run][email]
			if !ok || cur.UserID == "" || prev != cur {
				break
			}
			since = started[run]
		}
		out[email] = since
	}
	return out, nil
}
