// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

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
	var one int
	if err := st.DB().QueryRow(`SELECT 1 FROM sqlite_master WHERE type='table' AND name='resources' LIMIT 1`).Scan(&one); err != nil {
		_ = st.Close()
		return nil, false
	}
	return st, true
}
