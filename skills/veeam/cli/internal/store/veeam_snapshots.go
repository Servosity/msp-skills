// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package store

import (
	"context"
	"database/sql"
	"time"
)

// LatestUsageSnapshot returns the most recent recorded used-points value for an
// organization captured strictly before `before`. ok is false when no prior
// snapshot exists, which the caller treats as "no delta available yet".
func (s *Store) LatestUsageSnapshot(ctx context.Context, orgUID string, before time.Time) (float64, time.Time, bool, error) {
	var used float64
	var capturedAt string
	err := s.DB().QueryRowContext(ctx,
		`SELECT used_points, captured_at FROM veeam_usage_snapshots
		 WHERE org_uid = ? AND captured_at < ?
		 ORDER BY captured_at DESC LIMIT 1`,
		orgUID, before.UTC().Format(time.RFC3339Nano)).Scan(&used, &capturedAt)
	if err == sql.ErrNoRows {
		return 0, time.Time{}, false, nil
	}
	if err != nil {
		return 0, time.Time{}, false, err
	}
	t, _ := time.Parse(time.RFC3339Nano, capturedAt)
	return used, t, true, nil
}

// RecordUsageSnapshot stores a used-points sample for an organization at the
// given time. Idempotent on (org_uid, captured_at).
func (s *Store) RecordUsageSnapshot(ctx context.Context, orgUID string, usedPoints float64, at time.Time) error {
	_, err := s.DB().ExecContext(ctx,
		`INSERT OR REPLACE INTO veeam_usage_snapshots (org_uid, captured_at, used_points)
		 VALUES (?, ?, ?)`,
		orgUID, at.UTC().Format(time.RFC3339Nano), usedPoints)
	return err
}
