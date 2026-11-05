// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"os"
	"time"

	"hubspot-pp-cli/internal/store"
)

// hsOpenStore opens the local mirror read-only when the schema already exists
// (no write lock, so parallel read-only novel commands don't collide with
// SQLITE_BUSY), else read-write-migrate, with a short retry to ride out the
// first-run create/migrate race. Used by the pure-read novel commands.
func hsOpenStore(ctx context.Context, dbPath string) (*store.Store, error) {
	var lastErr error
	for attempt := 0; attempt < 8; attempt++ {
		if st, ok := hsTryReadOnlyMigrated(dbPath); ok {
			return st, nil
		}
		st, err := store.OpenWithContext(ctx, dbPath)
		if err == nil {
			return st, nil
		}
		lastErr = err
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		time.Sleep(time.Duration(20*(attempt+1)) * time.Millisecond)
	}
	return nil, lastErr
}

func hsTryReadOnlyMigrated(path string) (*store.Store, bool) {
	if path == "" {
		return nil, false
	}
	if _, err := os.Stat(path); err != nil {
		return nil, false
	}
	st, err := store.OpenReadOnly(path)
	if err != nil {
		return nil, false
	}
	// A store an older binary migrated lacks the newest extras table; it takes
	// the read-write-migrate path instead of failing on a missing table.
	var n int
	if err := st.DB().QueryRow(`SELECT (SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name IN ('resources', 'hubspot_pipeline_object_types')) + (SELECT COUNT(*) FROM pragma_table_xinfo('hubspot_property_history') WHERE name = 'snapshot_at')`).Scan(&n); err != nil || n != 3 {
		_ = st.Close()
		return nil, false
	}
	return st, true
}
