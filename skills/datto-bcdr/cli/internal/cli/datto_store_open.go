// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"os"
	"time"

	"datto-bcdr-pp-cli/internal/store"
)

// nvOpenStore opens the local mirror for the read-only novel commands. When the
// mirror already exists it opens read-only — a read-only handle takes no write
// lock, so several novel commands can run concurrently against the same mirror
// without the SQLITE_BUSY the scorecard's parallel sample probe surfaced. When
// the mirror is absent it is created and migrated (read-write). A short retry
// resolves the first-run race where several novels try to create the DB at
// once: the loser re-checks, finds the winner's file, and opens it read-only.
//
// The returned store is always non-nil on a nil error, so callers keep their
// existing `defer db.Close()` and `db.DB()` usage unchanged.
func nvOpenStore(ctx context.Context, dbPath string) (*store.Store, error) {
	if dbPath == "" {
		dbPath = defaultDBPath("datto-bcdr-cli")
	}
	var lastErr error
	for attempt := 0; attempt < 6; attempt++ {
		if _, statErr := os.Stat(dbPath); statErr == nil {
			s, err := store.OpenReadOnly(dbPath)
			if err == nil {
				return s, nil
			}
			lastErr = err
		} else {
			s, err := store.OpenWithContext(ctx, dbPath)
			if err == nil {
				return s, nil
			}
			lastErr = err
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		time.Sleep(time.Duration(20*(attempt+1)) * time.Millisecond)
	}
	return nil, lastErr
}
