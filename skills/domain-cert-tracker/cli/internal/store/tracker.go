// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): the client inventory, sweep observations,
// Certificate Transparency history and vendor_events tables the novel
// commands read and write. Created from migrateExtras (extras.go).
//
// vendor_events follows the zammad connector's shape so it is ready for MCP
// Events (SEP-3415, draft) without implementing it: seq is the cursor,
// event_id the stable id (UNIQUE, so a re-run sweep never repeats an event),
// type the event name, data the minimal before/after payload.

package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

var trackerMigrations = []string{
	`CREATE TABLE IF NOT EXISTS tracker_clients (
		name TEXT PRIMARY KEY,
		created_at TEXT NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS tracker_targets (
		client TEXT NOT NULL,
		kind TEXT NOT NULL CHECK (kind IN ('domain', 'host')),
		value TEXT NOT NULL,
		port INTEGER NOT NULL DEFAULT 443,
		PRIMARY KEY (client, kind, value, port)
	)`,
	`CREATE TABLE IF NOT EXISTS tracker_sweeps (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		started_at TEXT NOT NULL,
		finished_at TEXT NOT NULL DEFAULT '',
		client_filter TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL DEFAULT 'running'
	)`,
	`CREATE TABLE IF NOT EXISTS tracker_observations (
		sweep_id INTEGER NOT NULL,
		kind TEXT NOT NULL CHECK (kind IN ('rdap', 'ct', 'tls', 'dns')),
		subject TEXT NOT NULL,
		ok INTEGER NOT NULL,
		error TEXT NOT NULL DEFAULT '',
		data TEXT NOT NULL DEFAULT '{}',
		observed_at TEXT NOT NULL,
		PRIMARY KEY (sweep_id, kind, subject)
	)`,
	`CREATE INDEX IF NOT EXISTS idx_tracker_obs_subject ON tracker_observations(kind, subject, sweep_id)`,
	`CREATE TABLE IF NOT EXISTS tracker_ct_certs (
		domain TEXT NOT NULL,
		crt_id INTEGER NOT NULL,
		issuer_ca_id INTEGER NOT NULL DEFAULT 0,
		issuer_name TEXT NOT NULL DEFAULT '',
		common_name TEXT NOT NULL DEFAULT '',
		names TEXT NOT NULL DEFAULT '[]',
		not_before TEXT NOT NULL DEFAULT '',
		not_after TEXT NOT NULL DEFAULT '',
		serial TEXT NOT NULL DEFAULT '',
		first_seen_sweep INTEGER NOT NULL,
		PRIMARY KEY (domain, crt_id)
	)`,
	// The DNS names read from each certificate's own DER, downloaded from
	// crt.sh once per crt.sh id. crt.sh's search answer names only the
	// identities that matched the query, so tracker_ct_certs.names is not a
	// certificate's SAN list; a row here with status 'ok' is. A failed
	// download is kept as status 'failed' so later sweeps retry it after
	// certificates never tried.
	`CREATE TABLE IF NOT EXISTS tracker_cert_names (
		crt_id INTEGER PRIMARY KEY,
		status TEXT NOT NULL CHECK (status IN ('ok', 'failed')),
		names TEXT NOT NULL DEFAULT '[]',
		error TEXT NOT NULL DEFAULT '',
		checked_at TEXT NOT NULL
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
	// Which vendor_events seqs each sweep raised, so report can show one
	// sweep's changes without re-deriving them.
	`CREATE TABLE IF NOT EXISTS tracker_sweep_events (
		sweep_id INTEGER PRIMARY KEY,
		from_seq INTEGER NOT NULL,
		to_seq INTEGER NOT NULL
	)`,
}

// Target is one inventoried domain or host.
type Target struct {
	Client string `json:"client"`
	Kind   string `json:"kind"`
	Value  string `json:"value"`
	Port   int    `json:"port"`
}

// Client is one inventory entry with its targets.
type Client struct {
	Name    string   `json:"name"`
	Domains []string `json:"domains"`
	Hosts   []string `json:"hosts"`
	Targets []Target `json:"-"`
}

func nowText() string { return time.Now().UTC().Format(time.RFC3339Nano) }

func missingTable(err error) bool {
	return err != nil && strings.Contains(err.Error(), "no such table")
}

// AddTargets creates client if needed and adds every target (idempotent).
func (s *Store) AddTargets(ctx context.Context, client string, targets []Target) error {
	s.lockForWrite()
	defer s.unlockAfterWrite()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO tracker_clients (name, created_at) VALUES (?, ?)`, client, nowText()); err != nil {
		return err
	}
	for _, t := range targets {
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO tracker_targets (client, kind, value, port) VALUES (?, ?, ?, ?)`,
			client, t.Kind, t.Value, t.Port); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// RemoveClient deletes a client and its targets. It reports whether the
// client existed.
func (s *Store) RemoveClient(ctx context.Context, client string) (bool, error) {
	s.lockForWrite()
	defer s.unlockAfterWrite()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM tracker_targets WHERE client = ?`, client); err != nil {
		return false, err
	}
	res, err := tx.ExecContext(ctx, `DELETE FROM tracker_clients WHERE name = ?`, client)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, tx.Commit()
}

// RemoveTargets deletes the named targets from client and reports how many
// existed.
func (s *Store) RemoveTargets(ctx context.Context, client string, targets []Target) (int, error) {
	s.lockForWrite()
	defer s.unlockAfterWrite()
	total := 0
	for _, t := range targets {
		res, err := s.db.ExecContext(ctx, `DELETE FROM tracker_targets WHERE client = ? AND kind = ? AND value = ? AND port = ?`,
			client, t.Kind, t.Value, t.Port)
		if err != nil {
			return total, err
		}
		n, _ := res.RowsAffected()
		total += int(n)
	}
	return total, nil
}

// ListClients returns every client with its targets, sorted by name. A store
// that never had an inventory has no tables yet: that reads as no clients.
func (s *Store) ListClients(ctx context.Context) ([]Client, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT c.name, COALESCE(t.kind, ''), COALESCE(t.value, ''), COALESCE(t.port, 443)
		FROM tracker_clients c LEFT JOIN tracker_targets t ON t.client = c.name
		ORDER BY c.name, t.kind, t.value, t.port`)
	if err != nil {
		if missingTable(err) {
			return []Client{}, nil
		}
		return nil, err
	}
	defer rows.Close()
	out := []Client{}
	for rows.Next() {
		var name, kind, value string
		var port int
		if err := rows.Scan(&name, &kind, &value, &port); err != nil {
			return nil, err
		}
		if len(out) == 0 || out[len(out)-1].Name != name {
			out = append(out, Client{Name: name, Domains: []string{}, Hosts: []string{}})
		}
		c := &out[len(out)-1]
		switch kind {
		case "domain":
			c.Domains = append(c.Domains, value)
		case "host":
			label := value
			if port != 443 {
				label = fmt.Sprintf("%s:%d", value, port)
			}
			c.Hosts = append(c.Hosts, label)
		default:
			continue
		}
		c.Targets = append(c.Targets, Target{Client: name, Kind: kind, Value: value, Port: port})
	}
	return out, rows.Err()
}

// Sweep is one sweep run.
type Sweep struct {
	ID           int64  `json:"id"`
	StartedAt    string `json:"started_at"`
	FinishedAt   string `json:"finished_at"`
	ClientFilter string `json:"client_filter"`
	Status       string `json:"status"`
}

// BeginSweep records a new running sweep and returns its id.
func (s *Store) BeginSweep(ctx context.Context, clientFilter string) (int64, error) {
	s.lockForWrite()
	defer s.unlockAfterWrite()
	res, err := s.db.ExecContext(ctx, `INSERT INTO tracker_sweeps (started_at, client_filter) VALUES (?, ?)`, nowText(), clientFilter)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// FinishSweep marks a sweep complete with status.
func (s *Store) FinishSweep(ctx context.Context, id int64, status string) error {
	s.lockForWrite()
	defer s.unlockAfterWrite()
	_, err := s.db.ExecContext(ctx, `UPDATE tracker_sweeps SET finished_at = ?, status = ? WHERE id = ?`, nowText(), status, id)
	return err
}

// LatestSweep returns the sweep that finished last: by its finish time,
// not its id, since sweeps overlap (handfixes.json renewal-probe-order). A
// finish time that does not parse ranks below one that does; a tie goes to
// the higher id.
func (s *Store) LatestSweep(ctx context.Context) (Sweep, bool, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, started_at, finished_at, client_filter, status FROM tracker_sweeps
		WHERE finished_at != ''`)
	if missingTable(err) {
		return Sweep{}, false, nil
	}
	if err != nil {
		return Sweep{}, false, err
	}
	defer rows.Close()
	var best Sweep
	var bestAt time.Time
	found := false
	for rows.Next() {
		var sw Sweep
		if err := rows.Scan(&sw.ID, &sw.StartedAt, &sw.FinishedAt, &sw.ClientFilter, &sw.Status); err != nil {
			return Sweep{}, false, err
		}
		at, _ := time.Parse(time.RFC3339Nano, sw.FinishedAt)
		if !found || at.After(bestAt) || at.Equal(bestAt) && sw.ID > best.ID {
			best, bestAt, found = sw, at, true
		}
	}
	return best, found, rows.Err()
}

// Observation is one stored check result.
type Observation struct {
	SweepID    int64           `json:"sweep_id"`
	Kind       string          `json:"kind"`
	Subject    string          `json:"subject"`
	OK         bool            `json:"ok"`
	Error      string          `json:"error,omitempty"`
	Data       json.RawMessage `json:"data"`
	ObservedAt string          `json:"observed_at"`
}

// PutObservation stores one result for the sweep (replacing a retry's).
// ObservedAt is the time the check ran; empty means now.
func (s *Store) PutObservation(ctx context.Context, o Observation) error {
	data := string(o.Data)
	if data == "" {
		data = "{}"
	}
	observed := o.ObservedAt
	if observed == "" {
		observed = nowText()
	}
	s.lockForWrite()
	defer s.unlockAfterWrite()
	_, err := s.db.ExecContext(ctx, `INSERT OR REPLACE INTO tracker_observations (sweep_id, kind, subject, ok, error, data, observed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, o.SweepID, o.Kind, o.Subject, o.OK, o.Error, data, observed)
	return err
}

// ObservationTime returns when kind/subject was checked in sweepID.
func (s *Store) ObservationTime(ctx context.Context, sweepID int64, kind, subject string) (string, bool, error) {
	var at string
	err := s.db.QueryRowContext(ctx, `SELECT observed_at FROM tracker_observations WHERE sweep_id = ? AND kind = ? AND subject = ?`,
		sweepID, kind, subject).Scan(&at)
	if errors.Is(err, sql.ErrNoRows) || missingTable(err) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return at, true, nil
}

// LatestObservation returns the observation of kind/subject whose check ran
// last (0 beforeSweep means any sweep; otherwise sweeps before it). okOnly
// restricts it to successful observations. "Last" is by the time the check
// ran, never by sweep id: sweeps can overlap, and a later sweep's check can
// run before an earlier sweep's (handfixes.json renewal-probe-order).
func (s *Store) LatestObservation(ctx context.Context, kind, subject string, beforeSweep int64, okOnly bool) (Observation, bool, error) {
	return s.latestObservation(ctx, kind, subject, okOnly, func(sweep int64, _ time.Time, _ bool) bool {
		return beforeSweep <= 0 || sweep < beforeSweep
	})
}

// LatestObservationBefore returns the observation of kind/subject whose
// check ran last before at, from any sweep but exceptSweep: the state a
// check that ran at at changed from. A check at the same instant, or with
// no recorded time, counts as before only from an earlier sweep.
func (s *Store) LatestObservationBefore(ctx context.Context, kind, subject string, at time.Time, exceptSweep int64, okOnly bool) (Observation, bool, error) {
	return s.latestObservation(ctx, kind, subject, okOnly, func(sweep int64, t time.Time, dated bool) bool {
		if sweep == exceptSweep {
			return false
		}
		if dated && !t.Equal(at) {
			return t.Before(at)
		}
		return sweep < exceptSweep
	})
}

// latestObservation picks, among the rows keep accepts, the one whose
// observed_at is latest (parsed: RFC3339Nano text does not sort), ties and
// undated rows ordered by sweep id below every dated one.
func (s *Store) latestObservation(ctx context.Context, kind, subject string, okOnly bool, keep func(sweep int64, at time.Time, dated bool) bool) (Observation, bool, error) {
	q := `SELECT sweep_id, observed_at FROM tracker_observations WHERE kind = ? AND subject = ?`
	if okOnly {
		q += ` AND ok = 1`
	}
	rows, err := s.db.QueryContext(ctx, q, kind, subject)
	if missingTable(err) {
		return Observation{}, false, nil
	}
	if err != nil {
		return Observation{}, false, err
	}
	var best int64
	var bestAt time.Time
	found, bestDated := false, false
	for rows.Next() {
		var sweep int64
		var at string
		if err := rows.Scan(&sweep, &at); err != nil {
			_ = rows.Close()
			return Observation{}, false, err
		}
		t, perr := time.Parse(time.RFC3339Nano, at)
		dated := perr == nil
		if !keep(sweep, t, dated) {
			continue
		}
		better := !found
		switch {
		case !found:
		case dated != bestDated:
			better = dated
		case dated && !t.Equal(bestAt):
			better = t.After(bestAt)
		default:
			better = sweep > best
		}
		if better {
			best, bestAt, bestDated, found = sweep, t, dated, true
		}
	}
	if err := rows.Close(); err != nil {
		return Observation{}, false, err
	}
	if err := rows.Err(); err != nil {
		return Observation{}, false, err
	}
	if !found {
		return Observation{}, false, nil
	}
	var o Observation
	var data string
	err = s.db.QueryRowContext(ctx, `SELECT sweep_id, kind, subject, ok, error, data, observed_at FROM tracker_observations
		WHERE sweep_id = ? AND kind = ? AND subject = ?`, best, kind, subject).Scan(&o.SweepID, &o.Kind, &o.Subject, &o.OK, &o.Error, &data, &o.ObservedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Observation{}, false, nil
	}
	if err != nil {
		return Observation{}, false, err
	}
	o.Data = json.RawMessage(data)
	return o, true, nil
}

// Where a certificate's name list came from.
const (
	// NamesFromCertificate: the SAN list read from the certificate's own
	// DER, checked against the crt.sh row. The only complete name list.
	NamesFromCertificate = "certificate"
	// NamesFromCrtSh: crt.sh's search answer, which names only the
	// identities that matched the query. Never a complete SAN list.
	NamesFromCrtSh = "crtsh"
)

// CTCertRow is one stored crt.sh certificate for a domain.
type CTCertRow struct {
	Domain     string   `json:"domain"`
	CrtID      int64    `json:"crt_id"`
	IssuerCAID int64    `json:"issuer_ca_id"`
	IssuerName string   `json:"issuer_name"`
	CommonName string   `json:"common_name"`
	Names      []string `json:"names"`
	// NamesSource is NamesFromCertificate only when Names is the SAN list
	// read from the certificate itself; anything else is a partial list.
	NamesSource string `json:"names_source"`
	// NamesError says why the certificate's own names are not known.
	NamesError     string `json:"names_error,omitempty"`
	NotBefore      string `json:"not_before"`
	NotAfter       string `json:"not_after"`
	Serial         string `json:"serial"`
	FirstSeenSweep int64  `json:"first_seen_sweep"`
}

// Cert-name record statuses.
const (
	CertNamesOK     = "ok"
	CertNamesFailed = "failed"
)

// CertNames is the result of reading one certificate's names from its DER.
type CertNames struct {
	CrtID     int64    `json:"crt_id"`
	Status    string   `json:"status"`
	Names     []string `json:"names"`
	Error     string   `json:"error,omitempty"`
	CheckedAt string   `json:"checked_at"`
}

// CertNamesFor returns the stored records for ids. A store whose schema
// predates tracker_cert_names has none.
func (s *Store) CertNamesFor(ctx context.Context, ids []int64) (map[int64]CertNames, error) {
	out := map[int64]CertNames{}
	if len(ids) == 0 {
		return out, nil
	}
	idList, err := json.Marshal(ids)
	if err != nil {
		return nil, err
	}
	// The ids travel as one JSON-array parameter, so the query text is constant.
	rows, err := s.db.QueryContext(ctx, `SELECT crt_id, status, names, error, checked_at FROM tracker_cert_names WHERE crt_id IN (SELECT value FROM json_each(?))`, string(idList))
	if err != nil {
		if missingTable(err) {
			return out, nil
		}
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var c CertNames
		var names string
		if err := rows.Scan(&c.CrtID, &c.Status, &names, &c.Error, &c.CheckedAt); err != nil {
			return nil, err
		}
		if json.Unmarshal([]byte(names), &c.Names) != nil {
			c.Names = nil
		}
		out[c.CrtID] = c
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// PutCertNames records download results. A verified ('ok') record is final:
// a later failure never replaces it.
func (s *Store) PutCertNames(ctx context.Context, recs []CertNames) error {
	if len(recs) == 0 {
		return nil
	}
	s.lockForWrite()
	defer s.unlockAfterWrite()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, r := range recs {
		if r.Status != CertNamesOK && r.Status != CertNamesFailed {
			return fmt.Errorf("cert names for crt.sh id %d: bad status %q", r.CrtID, r.Status)
		}
		names, _ := json.Marshal(r.Names)
		if r.Names == nil {
			names = []byte("[]")
		}
		at := r.CheckedAt
		if at == "" {
			at = nowText()
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO tracker_cert_names (crt_id, status, names, error, checked_at) VALUES (?, ?, ?, ?, ?)
			ON CONFLICT(crt_id) DO UPDATE SET status = excluded.status, names = excluded.names, error = excluded.error, checked_at = excluded.checked_at
			WHERE tracker_cert_names.status != 'ok'`, r.CrtID, r.Status, string(names), r.Error, at); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// AddCTCerts stores certificates not yet seen for their domain and returns
// the ones that were new.
func (s *Store) AddCTCerts(ctx context.Context, sweepID int64, rows []CTCertRow) ([]CTCertRow, error) {
	s.lockForWrite()
	defer s.unlockAfterWrite()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	var fresh []CTCertRow
	for _, r := range rows {
		names, _ := json.Marshal(r.Names)
		res, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO tracker_ct_certs
			(domain, crt_id, issuer_ca_id, issuer_name, common_name, names, not_before, not_after, serial, first_seen_sweep)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			r.Domain, r.CrtID, r.IssuerCAID, r.IssuerName, r.CommonName, string(names), r.NotBefore, r.NotAfter, r.Serial, sweepID)
		if err != nil {
			return nil, err
		}
		if n, _ := res.RowsAffected(); n > 0 {
			r.FirstSeenSweep = sweepID
			fresh = append(fresh, r)
			continue
		}
		// Seen before. An overlapping sweep whose CT check ran earlier
		// than the stored first sighting's, though it stored later, is
		// the first sighting (handfixes.json renewal-probe-order).
		if err := keepEarliestCTSighting(ctx, tx, sweepID, r); err != nil {
			return nil, err
		}
	}
	return fresh, tx.Commit()
}

// ctObservationKind is tracker.KindCT: the CT check a sighting's time is.
const ctObservationKind = "ct"

// keepEarliestCTSighting moves r's first_seen_sweep to sweepID when this
// sweep's CT check of r.Domain ran strictly before the stored sweep's. An
// undated check on either side leaves the row as it is.
func keepEarliestCTSighting(ctx context.Context, tx *sql.Tx, sweepID int64, r CTCertRow) error {
	var stored int64
	if err := tx.QueryRowContext(ctx, `SELECT first_seen_sweep FROM tracker_ct_certs WHERE domain = ? AND crt_id = ?`,
		r.Domain, r.CrtID).Scan(&stored); err != nil {
		return err
	}
	if stored == sweepID {
		return nil
	}
	checkTime := func(sweep int64) (time.Time, bool, error) {
		var at string
		err := tx.QueryRowContext(ctx, `SELECT observed_at FROM tracker_observations WHERE sweep_id = ? AND kind = ? AND subject = ?`,
			sweep, ctObservationKind, r.Domain).Scan(&at)
		if errors.Is(err, sql.ErrNoRows) {
			return time.Time{}, false, nil
		}
		if err != nil {
			return time.Time{}, false, err
		}
		t, perr := time.Parse(time.RFC3339Nano, at)
		return t, perr == nil, nil
	}
	mine, ok1, err := checkTime(sweepID)
	if err != nil || !ok1 {
		return err
	}
	theirs, ok2, err := checkTime(stored)
	if err != nil || !ok2 || !mine.Before(theirs) {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE tracker_ct_certs SET first_seen_sweep = ? WHERE domain = ? AND crt_id = ?`, sweepID, r.Domain, r.CrtID)
	return err
}

// ListCTCerts returns the stored certificates for domain ("" for every
// domain), oldest crt.sh id first. A certificate whose own names were read
// ('ok' in tracker_cert_names) carries that SAN list with NamesSource
// NamesFromCertificate; every other row carries crt.sh's matched names
// with NamesSource NamesFromCrtSh.
func (s *Store) ListCTCerts(ctx context.Context, domain string) ([]CTCertRow, error) {
	// A read-only open of a store created before tracker_cert_names runs no
	// migration: its certificates are all partial, never missing.
	joined, err := s.hasTable(ctx, "tracker_cert_names")
	if err != nil {
		return nil, err
	}
	q := `SELECT c.domain, c.crt_id, c.issuer_ca_id, c.issuer_name, c.common_name, c.names, c.not_before, c.not_after, c.serial, c.first_seen_sweep,
		'', '', '' FROM tracker_ct_certs c`
	if joined {
		q = `SELECT c.domain, c.crt_id, c.issuer_ca_id, c.issuer_name, c.common_name, c.names, c.not_before, c.not_after, c.serial, c.first_seen_sweep,
			COALESCE(n.status, ''), COALESCE(n.names, ''), COALESCE(n.error, '')
			FROM tracker_ct_certs c LEFT JOIN tracker_cert_names n ON n.crt_id = c.crt_id`
	}
	var args []any
	if domain != "" {
		q += ` WHERE c.domain = ?`
		args = append(args, domain)
	}
	q += ` ORDER BY c.domain, c.crt_id`
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		if missingTable(err) {
			return []CTCertRow{}, nil
		}
		return nil, err
	}
	defer rows.Close()
	out := []CTCertRow{}
	for rows.Next() {
		var r CTCertRow
		var names, nStatus, nNames, nErr string
		if err := rows.Scan(&r.Domain, &r.CrtID, &r.IssuerCAID, &r.IssuerName, &r.CommonName, &names, &r.NotBefore, &r.NotAfter, &r.Serial, &r.FirstSeenSweep,
			&nStatus, &nNames, &nErr); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(names), &r.Names); err != nil || r.Names == nil {
			r.Names = []string{}
		}
		r.NamesSource = NamesFromCrtSh
		var full []string
		switch {
		case nStatus == CertNamesOK && json.Unmarshal([]byte(nNames), &full) == nil && len(full) > 0:
			r.Names, r.NamesSource = full, NamesFromCertificate
		case nStatus == CertNamesFailed:
			r.NamesError = nErr
		case nStatus == CertNamesOK:
			r.NamesError = "stored certificate names are unreadable"
		default:
			r.NamesError = "certificate not downloaded from crt.sh yet"
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) hasTable(ctx context.Context, name string) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, name).Scan(&n)
	return n > 0, err
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
		e.ReceivedAt = nowText()
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
// sweep has no table yet: that reads as no events.
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
		if missingTable(err) {
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

// EventsBetween returns the events with fromSeq < seq <= toSeq.
func (s *Store) EventsBetween(ctx context.Context, fromSeq, toSeq int64) ([]VendorEvent, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT seq, event_id, type, source, occurred_at, received_at, resource_type, resource_id, client, data
		FROM vendor_events WHERE seq > ? AND seq <= ? ORDER BY seq`, fromSeq, toSeq)
	if err != nil {
		if missingTable(err) {
			return []VendorEvent{}, nil
		}
		return nil, err
	}
	defer rows.Close()
	out := []VendorEvent{}
	for rows.Next() {
		var e VendorEvent
		var data string
		if err := rows.Scan(&e.Seq, &e.EventID, &e.Type, &e.Source, &e.OccurredAt, &e.ReceivedAt, &e.ResourceType, &e.ResourceID, &e.Client, &data); err != nil {
			return nil, err
		}
		e.Data = json.RawMessage(data)
		out = append(out, e)
	}
	return out, rows.Err()
}

// MaxEventSeq returns the highest event seq (0 when none).
func (s *Store) MaxEventSeq(ctx context.Context) (int64, error) {
	var n sql.NullInt64
	err := s.db.QueryRowContext(ctx, `SELECT MAX(seq) FROM vendor_events`).Scan(&n)
	if missingTable(err) {
		return 0, nil
	}
	return n.Int64, err
}

// SetSweepEventRange records which event seqs a sweep raised.
func (s *Store) SetSweepEventRange(ctx context.Context, sweepID, fromSeq, toSeq int64) error {
	s.lockForWrite()
	defer s.unlockAfterWrite()
	_, err := s.db.ExecContext(ctx, `INSERT OR REPLACE INTO tracker_sweep_events (sweep_id, from_seq, to_seq) VALUES (?, ?, ?)`, sweepID, fromSeq, toSeq)
	return err
}

// SweepEventRange returns the event seq range a sweep raised.
func (s *Store) SweepEventRange(ctx context.Context, sweepID int64) (int64, int64, bool, error) {
	var from, to int64
	err := s.db.QueryRowContext(ctx, `SELECT from_seq, to_seq FROM tracker_sweep_events WHERE sweep_id = ?`, sweepID).Scan(&from, &to)
	if errors.Is(err, sql.ErrNoRows) || missingTable(err) {
		return 0, 0, false, nil
	}
	return from, to, err == nil, err
}
