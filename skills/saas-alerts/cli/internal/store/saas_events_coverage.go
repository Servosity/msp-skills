// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/saas-alerts/handfixes.json
// under the "events-window-coverage" entry.

package store

import (
	"database/sql"
	"errors"
	"strings"
	"time"
)

const saasEventsCoverageDDL = `CREATE TABLE IF NOT EXISTS saas_events_coverage (
	id INTEGER PRIMARY KEY CHECK (id = 1),
	floor TEXT NOT NULL
)`

// SaveEventsCoverage records the earliest instant the locally synced events
// are complete from. The zero time means the full history the API returned.
func (s *Store) SaveEventsCoverage(floor time.Time) error {
	s.lockForWrite()
	defer s.unlockAfterWrite()
	if _, err := s.db.Exec(saasEventsCoverageDDL); err != nil {
		return err
	}
	value := ""
	if !floor.IsZero() {
		value = floor.UTC().Format(time.RFC3339)
	}
	_, err := s.db.Exec(`INSERT INTO saas_events_coverage (id, floor) VALUES (1, ?)
		ON CONFLICT(id) DO UPDATE SET floor = excluded.floor`, value)
	return err
}

// EventsCoverage returns the recorded coverage floor. ok is false when no
// completed events sync has recorded one; a zero floor with ok means the full
// history.
func (s *Store) EventsCoverage() (floor time.Time, ok bool, err error) {
	var value string
	err = s.db.QueryRow(`SELECT floor FROM saas_events_coverage WHERE id = 1`).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) || (err != nil && strings.Contains(err.Error(), "no such table")) {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, err
	}
	if value == "" {
		return time.Time{}, true, nil
	}
	floor, err = time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, false, err
	}
	return floor, true, nil
}
