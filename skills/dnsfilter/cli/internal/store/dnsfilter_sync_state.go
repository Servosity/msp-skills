// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored (handfixes.json: sync-clear-never-marks-complete). A regen
// preserves this file by construction; the generated sync.go clear sites must
// call ClearSyncCursor instead of SaveSyncStateAt.

package store

// ClearSyncCursor resets a resource's resume cursor, incremental watermark and
// count before a --full or --latest-only walk. It marks the attempt
// incomplete, never complete: a walk that never runs (a usage error, Ctrl-C,
// a timeout, a SIGTERM from an MCP host) must not leave a collection reading
// as synced with zero rows. Only the walk's own final save, after a proven
// complete walk, sets last_attempt_complete = 1.
func (s *Store) ClearSyncCursor(resourceType string) error {
	s.lockForWrite()
	defer s.unlockAfterWrite()
	_, err := s.db.Exec(
		`INSERT INTO sync_state (resource_type, last_cursor, last_synced_at, total_count, last_attempt_complete)
		 VALUES (?, '', NULL, 0, 0)
		 ON CONFLICT(resource_type) DO UPDATE SET last_cursor = '',
		 last_synced_at = NULL, total_count = 0, last_attempt_complete = 0`,
		resourceType,
	)
	return err
}
