// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): the vendor_events table that `issues watch`
// writes and `events list` reads. Pinned by this connector's handfixes.json
// under the "issues-watch-events" entry. Created from migrateExtras
// (extras.go). Same table, row and query shape as the zammad connector's
// vendor_events.go, so the two connectors' events read alike.
//
// Not learn_events: events.go owns that name for the learn-loop telemetry.
// The row shape is ready for MCP Events (SEP-3415, draft): seq is the
// cursor, event_id the stable eventId (never the per-delivery id), type the
// event name, data the minimal payload. A later events/poll handler is the
// SELECT in VendorEventsSince.

package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
)

// vendorEventsMigrations are idempotent; migrateExtras runs them on every
// writable open. event_id is UNIQUE so a webhook row and a poll row for
// the same upstream change are one row; delivery_id is UNIQUE so a replayed
// delivery is dropped (sync rows carry NULL, which UNIQUE allows many of).
var vendorEventsMigrations = []string{
	`CREATE TABLE IF NOT EXISTS vendor_events (
		seq INTEGER PRIMARY KEY AUTOINCREMENT,
		event_id TEXT NOT NULL UNIQUE,
		type TEXT NOT NULL,
		source TEXT NOT NULL CHECK (source IN ('webhook', 'sync', 'poll')),
		occurred_at TEXT NOT NULL,
		received_at TEXT NOT NULL,
		resource_type TEXT NOT NULL,
		resource_id TEXT NOT NULL,
		delivery_id TEXT UNIQUE,
		verified INTEGER NOT NULL DEFAULT 0,
		data TEXT NOT NULL DEFAULT '{}'
	)`,
	`CREATE INDEX IF NOT EXISTS idx_vendor_events_type_seq ON vendor_events(type, seq)`,
	// One row per source: when it last recorded. A missing row for the issues
	// poll means its next run records a baseline, not events.
	`CREATE TABLE IF NOT EXISTS vendor_event_watermarks (
		source TEXT PRIMARY KEY,
		since TEXT NOT NULL
	)`,
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
	DeliveryID   string          `json:"delivery_id,omitempty"`
	Verified     bool            `json:"verified"`
	Data         json.RawMessage `json:"data"`
}

const insertVendorEventSQL = `INSERT OR IGNORE INTO vendor_events
	(event_id, type, source, occurred_at, received_at, resource_type, resource_id, delivery_id, verified, data)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

// InsertVendorEvent writes e unless its event_id or delivery_id is already
// recorded. It returns the new seq, or 0 for a duplicate.
func (s *Store) InsertVendorEvent(ctx context.Context, e VendorEvent) (int64, error) {
	var delivery any
	if e.DeliveryID != "" {
		delivery = e.DeliveryID
	}
	data := string(e.Data)
	if data == "" {
		data = "{}"
	}
	s.lockForWrite()
	defer s.unlockAfterWrite()
	res, err := s.db.ExecContext(ctx, insertVendorEventSQL,
		e.EventID, e.Type, e.Source, e.OccurredAt, e.ReceivedAt, e.ResourceType, e.ResourceID, delivery, e.Verified, data)
	if err != nil {
		return 0, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return 0, nil
	}
	return res.LastInsertId()
}

// VendorDeliverySeen reports whether a delivery id is already recorded.
func (s *Store) VendorDeliverySeen(ctx context.Context, deliveryID string) (bool, error) {
	var one int
	err := s.db.QueryRowContext(ctx, `SELECT 1 FROM vendor_events WHERE delivery_id = ?`, deliveryID).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

// VendorEventsSince returns up to limit rows after cursor seq, oldest first,
// optionally of one type, and whether more remain. A store that has never
// run a watcher has no table yet: that reads as no events.
func (s *Store) VendorEventsSince(ctx context.Context, since int64, eventType string, limit int) ([]VendorEvent, bool, error) {
	q := `SELECT seq, event_id, type, source, occurred_at, received_at, resource_type, resource_id,
		COALESCE(delivery_id, ''), verified, data FROM vendor_events WHERE seq > ?`
	args := []any{since}
	if eventType != "" {
		q += ` AND type = ?`
		args = append(args, eventType)
	}
	q += ` ORDER BY seq LIMIT ?`
	args = append(args, limit+1)
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil, false, nil
		}
		return nil, false, err
	}
	defer rows.Close()
	var out []VendorEvent
	for rows.Next() {
		var e VendorEvent
		var data string
		if err := rows.Scan(&e.Seq, &e.EventID, &e.Type, &e.Source, &e.OccurredAt, &e.ReceivedAt,
			&e.ResourceType, &e.ResourceID, &e.DeliveryID, &e.Verified, &data); err != nil {
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

// VendorWatermark returns the time source last recorded, and false when it
// never has.
func (s *Store) VendorWatermark(ctx context.Context, source string) (string, bool, error) {
	var since string
	err := s.db.QueryRowContext(ctx, `SELECT since FROM vendor_event_watermarks WHERE source = ?`, source).Scan(&since)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	return since, err == nil, err
}

// SetVendorWatermark records the time source last recorded.
func (s *Store) SetVendorWatermark(ctx context.Context, source, since string) error {
	s.lockForWrite()
	defer s.unlockAfterWrite()
	_, err := s.db.ExecContext(ctx, `INSERT INTO vendor_event_watermarks (source, since) VALUES (?, ?)
		ON CONFLICT(source) DO UPDATE SET since = excluded.since`, source, since)
	return err
}
