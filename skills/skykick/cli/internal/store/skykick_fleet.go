// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored fleet store layer for skykick-cli (not generator-emitted).
//
// The fleet-* transcendence commands need per-subscription facet data
// (settings, retention, autodiscover, snapshot stats, mailboxes, sites,
// alerts) joined across every customer tenant, plus history so `drift` can
// diff two sync runs. The generator's generic resources table is per-entity
// and unversioned, so the fleet schema lives in its own run-versioned tables,
// created lazily by the commands that need them. Raw upstream JSON is always
// stored alongside extracted columns so `sql` users can reach unmodeled
// fields.
package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"skykick-pp-cli/internal/fleet"
)

// fleetRunsToKeep bounds history growth: PruneFleetRuns deletes everything
// older than the newest N runs. Two runs are enough for drift; five gives
// headroom for manual archaeology without unbounded growth.
const fleetRunsToKeep = 5

// EnsureFleetSchema creates the fleet tables when absent. Safe to call from
// every fleet command; SQLite IF NOT EXISTS makes it idempotent.
func (s *Store) EnsureFleetSchema(ctx context.Context) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS fleet_runs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			started_at TEXT NOT NULL,
			finished_at TEXT,
			subscriptions INTEGER NOT NULL DEFAULT 0,
			subscriptions_seen INTEGER NOT NULL DEFAULT 0,
			fetch_failures INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS fleet_subscriptions (
			run_id INTEGER NOT NULL,
			id TEXT NOT NULL,
			company_name TEXT,
			partner_id TEXT,
			raw TEXT,
			PRIMARY KEY (run_id, id)
		)`,
		`CREATE TABLE IF NOT EXISTS fleet_settings (
			run_id INTEGER NOT NULL,
			subscription_id TEXT NOT NULL,
			company_name TEXT,
			exchange_enabled INTEGER,
			sharepoint_enabled INTEGER,
			raw TEXT,
			PRIMARY KEY (run_id, subscription_id)
		)`,
		`CREATE TABLE IF NOT EXISTS fleet_retention (
			run_id INTEGER NOT NULL,
			subscription_id TEXT NOT NULL,
			exchange_days INTEGER,
			sharepoint_days INTEGER,
			raw TEXT,
			PRIMARY KEY (run_id, subscription_id)
		)`,
		`CREATE TABLE IF NOT EXISTS fleet_autodiscover (
			run_id INTEGER NOT NULL,
			subscription_id TEXT NOT NULL,
			exchange_on INTEGER,
			sharepoint_on INTEGER,
			raw TEXT,
			PRIMARY KEY (run_id, subscription_id)
		)`,
		`CREATE TABLE IF NOT EXISTS fleet_snapshot_stats (
			run_id INTEGER NOT NULL,
			subscription_id TEXT NOT NULL,
			mailbox TEXT NOT NULL,
			last_snapshot TEXT,
			raw TEXT,
			PRIMARY KEY (run_id, subscription_id, mailbox)
		)`,
		`CREATE TABLE IF NOT EXISTS fleet_mailboxes (
			run_id INTEGER NOT NULL,
			subscription_id TEXT NOT NULL,
			mailbox_id TEXT NOT NULL DEFAULT '',
			email TEXT NOT NULL DEFAULT '',
			enabled INTEGER,
			raw TEXT,
			PRIMARY KEY (run_id, subscription_id, mailbox_id, email)
		)`,
		`CREATE TABLE IF NOT EXISTS fleet_sites (
			run_id INTEGER NOT NULL,
			subscription_id TEXT NOT NULL,
			url TEXT NOT NULL,
			enabled INTEGER,
			raw TEXT,
			PRIMARY KEY (run_id, subscription_id, url)
		)`,
		`CREATE TABLE IF NOT EXISTS fleet_alerts (
			run_id INTEGER NOT NULL,
			subscription_id TEXT NOT NULL,
			alert_id TEXT NOT NULL,
			severity TEXT,
			status TEXT,
			description TEXT,
			created TEXT,
			raw TEXT,
			PRIMARY KEY (run_id, subscription_id, alert_id)
		)`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("creating fleet schema: %w", err)
		}
	}
	return nil
}

// BeginFleetRun opens a new sync-run record and returns its id. It first
// reclaims rows orphaned by abandoned runs (Ctrl-C mid-sync leaves facet rows
// under a run that never gets finished_at; those rows are invisible to the
// finished-run readers but linger for `sql` users). Only runs older than an
// hour are reclaimed so a hypothetical concurrent sync is not clobbered.
func (s *Store) BeginFleetRun(ctx context.Context) (int64, error) {
	cutoff := time.Now().UTC().Add(-1 * time.Hour).Format(time.RFC3339)
	tables := []string{
		"fleet_subscriptions", "fleet_settings", "fleet_retention",
		"fleet_autodiscover", "fleet_snapshot_stats", "fleet_mailboxes",
		"fleet_sites", "fleet_alerts",
	}
	for _, table := range tables {
		// #nosec G201 -- table is iterated from the compile-time `tables` slice
		// literal above (a fixed allowlist of fleet_* table names), never from
		// user input; the only user-influenced value (cutoff) is bound as ?.
		stmt := fmt.Sprintf(
			`DELETE FROM %s WHERE run_id IN (SELECT id FROM fleet_runs WHERE finished_at IS NULL AND started_at < ?)`, table)
		if _, err := s.db.ExecContext(ctx, stmt, cutoff); err != nil {
			return 0, fmt.Errorf("reclaiming abandoned run rows from %s: %w", table, err)
		}
	}
	if _, err := s.db.ExecContext(ctx,
		`DELETE FROM fleet_runs WHERE finished_at IS NULL AND started_at < ?`, cutoff); err != nil {
		return 0, fmt.Errorf("reclaiming abandoned runs: %w", err)
	}

	res, err := s.db.ExecContext(ctx,
		`INSERT INTO fleet_runs (started_at) VALUES (?)`,
		time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return 0, fmt.Errorf("starting fleet run: %w", err)
	}
	return res.LastInsertId()
}

// FinishFleetRun stamps completion stats on a run and prunes old runs.
// subscriptionsSeen is the upstream total BEFORE any --limit slicing; a run
// with subscriptions < subscriptionsSeen is partial, and drift refuses to
// treat its missing tenants as removed/added.
func (s *Store) FinishFleetRun(ctx context.Context, runID int64, subscriptions, subscriptionsSeen, fetchFailures int) error {
	if _, err := s.db.ExecContext(ctx,
		`UPDATE fleet_runs SET finished_at = ?, subscriptions = ?, subscriptions_seen = ?, fetch_failures = ? WHERE id = ?`,
		time.Now().UTC().Format(time.RFC3339), subscriptions, subscriptionsSeen, fetchFailures, runID); err != nil {
		return fmt.Errorf("finishing fleet run: %w", err)
	}
	return s.pruneFleetRuns(ctx, fleetRunsToKeep)
}

// pruneFleetRuns deletes all fleet data outside the newest keep runs.
func (s *Store) pruneFleetRuns(ctx context.Context, keep int) error {
	tables := []string{
		"fleet_subscriptions", "fleet_settings", "fleet_retention",
		"fleet_autodiscover", "fleet_snapshot_stats", "fleet_mailboxes",
		"fleet_sites", "fleet_alerts",
	}
	for _, table := range tables {
		// #nosec G201 -- table is iterated from the compile-time `tables` slice
		// literal above (a fixed allowlist of fleet_* table names), never from
		// user input; the only user-influenced value (keep) is bound as ?.
		stmt := fmt.Sprintf(
			`DELETE FROM %s WHERE run_id NOT IN (SELECT id FROM fleet_runs ORDER BY id DESC LIMIT ?)`, table)
		if _, err := s.db.ExecContext(ctx, stmt, keep); err != nil {
			return fmt.Errorf("pruning %s: %w", table, err)
		}
	}
	if _, err := s.db.ExecContext(ctx,
		`DELETE FROM fleet_runs WHERE id NOT IN (SELECT id FROM fleet_runs ORDER BY id DESC LIMIT ?)`, keep); err != nil {
		return fmt.Errorf("pruning fleet_runs: %w", err)
	}
	return nil
}

// FleetRun is one row of fleet_runs.
type FleetRun struct {
	ID                int64  `json:"id"`
	StartedAt         string `json:"started_at"`
	FinishedAt        string `json:"finished_at,omitempty"`
	Subscriptions     int    `json:"subscriptions"`
	SubscriptionsSeen int    `json:"subscriptions_seen"`
	FetchFailures     int    `json:"fetch_failures"`
}

// Partial reports whether the run synced fewer subscriptions than the
// upstream listed (a --limit'd or dogfood-curtailed sync).
func (r FleetRun) Partial() bool {
	return r.SubscriptionsSeen > 0 && r.Subscriptions < r.SubscriptionsSeen
}

// LatestFleetRuns returns the newest n FINISHED runs, newest first. Unfinished
// runs (crashed mid-sync) are excluded so analyzers never read half a fleet.
func (s *Store) LatestFleetRuns(ctx context.Context, n int) ([]FleetRun, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, started_at, COALESCE(finished_at, ''), subscriptions, subscriptions_seen, fetch_failures
		 FROM fleet_runs WHERE finished_at IS NOT NULL ORDER BY id DESC LIMIT ?`, n)
	if err != nil {
		return nil, fmt.Errorf("listing fleet runs: %w", err)
	}
	defer rows.Close()
	var runs []FleetRun
	for rows.Next() {
		var r FleetRun
		if err := rows.Scan(&r.ID, &r.StartedAt, &r.FinishedAt, &r.Subscriptions, &r.SubscriptionsSeen, &r.FetchFailures); err != nil {
			return nil, fmt.Errorf("scanning fleet run: %w", err)
		}
		runs = append(runs, r)
	}
	return runs, rows.Err()
}

// boolToNull maps tri-state *bool to SQL (NULL = unknown).
func boolToNull(b *bool) any {
	if b == nil {
		return nil
	}
	if *b {
		return 1
	}
	return 0
}

func intToNull(i *int) any {
	if i == nil {
		return nil
	}
	return *i
}

func timeToNull(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.UTC().Format(time.RFC3339)
}

// nullToBool maps SQL NULL-able ints back to tri-state *bool.
func nullToBool(v sql.NullInt64) *bool {
	if !v.Valid {
		return nil
	}
	b := v.Int64 != 0
	return &b
}

func nullToInt(v sql.NullInt64) *int {
	if !v.Valid {
		return nil
	}
	i := int(v.Int64)
	return &i
}

func nullToTime(v sql.NullString) *time.Time {
	if !v.Valid || v.String == "" {
		return nil
	}
	ts, err := time.Parse(time.RFC3339, v.String)
	if err != nil {
		return nil
	}
	return &ts
}

// InsertFleetState writes one subscription's parsed facets under runID inside
// a single transaction. Called once per subscription as the syncer's fan-out
// workers finish; partial fleet syncs therefore persist everything that
// succeeded even when later subscriptions fail.
func (s *Store) InsertFleetState(ctx context.Context, runID int64, sub fleet.Subscription, settings *fleet.Settings, retention *fleet.Retention, auto *fleet.Autodiscover, stats []fleet.MailboxStat, mailboxes []fleet.Mailbox, sites []fleet.Site, alerts []fleet.Alert) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning fleet insert: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx,
		`INSERT OR REPLACE INTO fleet_subscriptions (run_id, id, company_name, partner_id, raw) VALUES (?,?,?,?,?)`,
		runID, sub.ID, sub.CompanyName, sub.PartnerID, string(sub.Raw)); err != nil {
		return fmt.Errorf("inserting subscription %s: %w", sub.ID, err)
	}
	if settings != nil {
		if _, err := tx.ExecContext(ctx,
			`INSERT OR REPLACE INTO fleet_settings (run_id, subscription_id, company_name, exchange_enabled, sharepoint_enabled, raw) VALUES (?,?,?,?,?,?)`,
			runID, settings.SubscriptionID, settings.CompanyName, boolToNull(settings.ExchangeEnabled), boolToNull(settings.SharePointEnabled), string(settings.Raw)); err != nil {
			return fmt.Errorf("inserting settings %s: %w", sub.ID, err)
		}
	}
	if retention != nil {
		if _, err := tx.ExecContext(ctx,
			`INSERT OR REPLACE INTO fleet_retention (run_id, subscription_id, exchange_days, sharepoint_days, raw) VALUES (?,?,?,?,?)`,
			runID, retention.SubscriptionID, intToNull(retention.ExchangeDays), intToNull(retention.SharePointDays), string(retention.Raw)); err != nil {
			return fmt.Errorf("inserting retention %s: %w", sub.ID, err)
		}
	}
	if auto != nil {
		if _, err := tx.ExecContext(ctx,
			`INSERT OR REPLACE INTO fleet_autodiscover (run_id, subscription_id, exchange_on, sharepoint_on, raw) VALUES (?,?,?,?,?)`,
			runID, auto.SubscriptionID, boolToNull(auto.ExchangeOn), boolToNull(auto.SharePointOn), string(auto.Raw)); err != nil {
			return fmt.Errorf("inserting autodiscover %s: %w", sub.ID, err)
		}
	}
	for _, st := range stats {
		if _, err := tx.ExecContext(ctx,
			`INSERT OR REPLACE INTO fleet_snapshot_stats (run_id, subscription_id, mailbox, last_snapshot, raw) VALUES (?,?,?,?,?)`,
			runID, st.SubscriptionID, st.Mailbox, timeToNull(st.LastSnapshot), string(st.Raw)); err != nil {
			return fmt.Errorf("inserting snapshot stat %s/%s: %w", sub.ID, st.Mailbox, err)
		}
	}
	for _, mb := range mailboxes {
		if _, err := tx.ExecContext(ctx,
			`INSERT OR REPLACE INTO fleet_mailboxes (run_id, subscription_id, mailbox_id, email, enabled, raw) VALUES (?,?,?,?,?,?)`,
			runID, mb.SubscriptionID, mb.ID, mb.Email, boolToNull(mb.Enabled), string(mb.Raw)); err != nil {
			return fmt.Errorf("inserting mailbox %s/%s: %w", sub.ID, mb.Email, err)
		}
	}
	for _, site := range sites {
		if _, err := tx.ExecContext(ctx,
			`INSERT OR REPLACE INTO fleet_sites (run_id, subscription_id, url, enabled, raw) VALUES (?,?,?,?,?)`,
			runID, site.SubscriptionID, site.URL, boolToNull(site.Enabled), string(site.Raw)); err != nil {
			return fmt.Errorf("inserting site %s/%s: %w", sub.ID, site.URL, err)
		}
	}
	for _, al := range alerts {
		if _, err := tx.ExecContext(ctx,
			`INSERT OR REPLACE INTO fleet_alerts (run_id, subscription_id, alert_id, severity, status, description, created, raw) VALUES (?,?,?,?,?,?,?,?)`,
			runID, al.SubscriptionID, al.ID, al.Severity, al.Status, al.Description, timeToNull(al.Created), string(al.Raw)); err != nil {
			return fmt.Errorf("inserting alert %s/%s: %w", sub.ID, al.ID, err)
		}
	}
	return tx.Commit()
}

// LoadFleetState reads every facet for one run back into fleet types.
func (s *Store) LoadFleetState(ctx context.Context, runID int64) (fleet.FleetState, error) {
	var state fleet.FleetState

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, COALESCE(company_name,''), COALESCE(partner_id,''), COALESCE(raw,'') FROM fleet_subscriptions WHERE run_id = ?`, runID)
	if err != nil {
		return state, fmt.Errorf("loading subscriptions: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var sub fleet.Subscription
		var raw string
		if err := rows.Scan(&sub.ID, &sub.CompanyName, &sub.PartnerID, &raw); err != nil {
			return state, fmt.Errorf("scanning subscription: %w", err)
		}
		sub.Raw = json.RawMessage(raw)
		state.Subscriptions = append(state.Subscriptions, sub)
	}
	if err := rows.Err(); err != nil {
		return state, err
	}

	setRows, err := s.db.QueryContext(ctx,
		`SELECT subscription_id, COALESCE(company_name,''), exchange_enabled, sharepoint_enabled FROM fleet_settings WHERE run_id = ?`, runID)
	if err != nil {
		return state, fmt.Errorf("loading settings: %w", err)
	}
	defer setRows.Close()
	for setRows.Next() {
		var st fleet.Settings
		var exch, sp sql.NullInt64
		if err := setRows.Scan(&st.SubscriptionID, &st.CompanyName, &exch, &sp); err != nil {
			return state, fmt.Errorf("scanning settings: %w", err)
		}
		st.ExchangeEnabled = nullToBool(exch)
		st.SharePointEnabled = nullToBool(sp)
		state.Settings = append(state.Settings, st)
	}
	if err := setRows.Err(); err != nil {
		return state, err
	}

	retRows, err := s.db.QueryContext(ctx,
		`SELECT subscription_id, exchange_days, sharepoint_days FROM fleet_retention WHERE run_id = ?`, runID)
	if err != nil {
		return state, fmt.Errorf("loading retention: %w", err)
	}
	defer retRows.Close()
	for retRows.Next() {
		var r fleet.Retention
		var exch, sp sql.NullInt64
		if err := retRows.Scan(&r.SubscriptionID, &exch, &sp); err != nil {
			return state, fmt.Errorf("scanning retention: %w", err)
		}
		r.ExchangeDays = nullToInt(exch)
		r.SharePointDays = nullToInt(sp)
		state.Retention = append(state.Retention, r)
	}
	if err := retRows.Err(); err != nil {
		return state, err
	}

	autoRows, err := s.db.QueryContext(ctx,
		`SELECT subscription_id, exchange_on, sharepoint_on FROM fleet_autodiscover WHERE run_id = ?`, runID)
	if err != nil {
		return state, fmt.Errorf("loading autodiscover: %w", err)
	}
	defer autoRows.Close()
	for autoRows.Next() {
		var a fleet.Autodiscover
		var exch, sp sql.NullInt64
		if err := autoRows.Scan(&a.SubscriptionID, &exch, &sp); err != nil {
			return state, fmt.Errorf("scanning autodiscover: %w", err)
		}
		a.ExchangeOn = nullToBool(exch)
		a.SharePointOn = nullToBool(sp)
		state.Autodiscover = append(state.Autodiscover, a)
	}
	if err := autoRows.Err(); err != nil {
		return state, err
	}

	statRows, err := s.db.QueryContext(ctx,
		`SELECT subscription_id, mailbox, last_snapshot FROM fleet_snapshot_stats WHERE run_id = ?`, runID)
	if err != nil {
		return state, fmt.Errorf("loading snapshot stats: %w", err)
	}
	defer statRows.Close()
	for statRows.Next() {
		var st fleet.MailboxStat
		var snap sql.NullString
		if err := statRows.Scan(&st.SubscriptionID, &st.Mailbox, &snap); err != nil {
			return state, fmt.Errorf("scanning snapshot stat: %w", err)
		}
		st.LastSnapshot = nullToTime(snap)
		state.Stats = append(state.Stats, st)
	}
	if err := statRows.Err(); err != nil {
		return state, err
	}

	mbRows, err := s.db.QueryContext(ctx,
		`SELECT subscription_id, COALESCE(mailbox_id,''), COALESCE(email,''), enabled FROM fleet_mailboxes WHERE run_id = ?`, runID)
	if err != nil {
		return state, fmt.Errorf("loading mailboxes: %w", err)
	}
	defer mbRows.Close()
	for mbRows.Next() {
		var mb fleet.Mailbox
		var enabled sql.NullInt64
		if err := mbRows.Scan(&mb.SubscriptionID, &mb.ID, &mb.Email, &enabled); err != nil {
			return state, fmt.Errorf("scanning mailbox: %w", err)
		}
		mb.Enabled = nullToBool(enabled)
		state.Mailboxes = append(state.Mailboxes, mb)
	}
	if err := mbRows.Err(); err != nil {
		return state, err
	}

	siteRows, err := s.db.QueryContext(ctx,
		`SELECT subscription_id, url, enabled FROM fleet_sites WHERE run_id = ?`, runID)
	if err != nil {
		return state, fmt.Errorf("loading sites: %w", err)
	}
	defer siteRows.Close()
	for siteRows.Next() {
		var site fleet.Site
		var enabled sql.NullInt64
		if err := siteRows.Scan(&site.SubscriptionID, &site.URL, &enabled); err != nil {
			return state, fmt.Errorf("scanning site: %w", err)
		}
		site.Enabled = nullToBool(enabled)
		state.Sites = append(state.Sites, site)
	}
	if err := siteRows.Err(); err != nil {
		return state, err
	}

	alertRows, err := s.db.QueryContext(ctx,
		`SELECT subscription_id, alert_id, COALESCE(severity,''), COALESCE(status,''), COALESCE(description,''), created FROM fleet_alerts WHERE run_id = ?`, runID)
	if err != nil {
		return state, fmt.Errorf("loading alerts: %w", err)
	}
	defer alertRows.Close()
	for alertRows.Next() {
		var al fleet.Alert
		var created sql.NullString
		if err := alertRows.Scan(&al.SubscriptionID, &al.ID, &al.Severity, &al.Status, &al.Description, &created); err != nil {
			return state, fmt.Errorf("scanning alert: %w", err)
		}
		al.Created = nullToTime(created)
		state.Alerts = append(state.Alerts, al)
	}
	return state, alertRows.Err()
}
