// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package store

import (
	"context"
	"fmt"
	"time"
)

// veeamSnapshotLayout is a fixed-width UTC timestamp, so captured_at sorts
// chronologically as text (RFC3339Nano drops trailing zeros: ".1Z" > ".11Z").
const veeamSnapshotLayout = "2006-01-02T15:04:05.000000000Z07:00"

// LatestUsageSnapshot returns the most recent recorded used-points value for an
// organization captured strictly before `before`. ok is false when no prior
// snapshot exists, which the caller treats as "no delta available yet".
func (s *Store) LatestUsageSnapshot(ctx context.Context, orgUID string, before time.Time) (float64, time.Time, bool, error) {
	// Compare parsed times, not text: rows written before the fixed-width
	// layout carry RFC3339Nano, whose text does not sort by time.
	rows, err := s.DB().QueryContext(ctx,
		`SELECT used_points, captured_at FROM veeam_usage_snapshots WHERE org_uid = ?`, orgUID)
	if err != nil {
		return 0, time.Time{}, false, err
	}
	defer rows.Close()
	var best float64
	var bestAt time.Time
	found := false
	for rows.Next() {
		var used float64
		var capturedAt string
		if err := rows.Scan(&used, &capturedAt); err != nil {
			return 0, time.Time{}, false, err
		}
		at, err := time.Parse(time.RFC3339Nano, capturedAt)
		if err != nil {
			return 0, time.Time{}, false, fmt.Errorf("usage snapshot for %s has an unreadable time %q: %w", orgUID, capturedAt, err)
		}
		if at.Before(before) && (!found || at.After(bestAt)) {
			best, bestAt, found = used, at, true
		}
	}
	if err := rows.Err(); err != nil {
		return 0, time.Time{}, false, err
	}
	return best, bestAt, found, nil
}

// RecordUsageSnapshot stores a used-points sample for an organization at the
// given time. Idempotent on (org_uid, captured_at).
func (s *Store) RecordUsageSnapshot(ctx context.Context, orgUID string, usedPoints float64, at time.Time) error {
	_, err := s.DB().ExecContext(ctx,
		`INSERT OR REPLACE INTO veeam_usage_snapshots (org_uid, captured_at, used_points)
		 VALUES (?, ?, ?)`,
		orgUID, at.UTC().Format(veeamSnapshotLayout), usedPoints)
	return err
}
