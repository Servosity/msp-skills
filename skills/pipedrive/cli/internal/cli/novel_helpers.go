// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"

	"pipedrive-pp-cli/internal/store"
)

// novelDBPath resolves the local-store path for a read-only intelligence
// command (stale, forecast, aging, digest, changes, dupes, leaderboard),
// falling back to the default when --db is empty. The store creates its tables
// on open, so the commands are safe to run before the first `sync` — queries
// simply return no rows. Each command opens the store itself via
// store.OpenWithContext(ctx, novelDBPath(dbPath)).
func novelDBPath(dbPath string) string {
	if dbPath == "" {
		return defaultDBPath("pipedrive-cli")
	}
	return dbPath
}

// emitNovel renders v as JSON (honoring --json/--agent/--select/--compact and
// non-TTY pipes) or, for an interactive terminal, calls renderHuman.
func emitNovel(cmd *cobra.Command, flags *rootFlags, v any, renderHuman func(io.Writer)) error {
	w := cmd.OutOrStdout()
	if flags.asJSON || flags.agent || (!isTerminal(w) && !humanFriendly) {
		return printJSONFiltered(w, v, flags)
	}
	renderHuman(w)
	return nil
}

// tableHasColumn reports whether table has a column named col. Defensive for
// the changes command, which iterates entities whose update_time column may or
// may not be present in a given store schema variant.
func tableHasColumn(ctx context.Context, db *sql.DB, table, col string) bool {
	rows, err := db.QueryContext(ctx, fmt.Sprintf("PRAGMA table_info(%q)", table))
	if err != nil {
		return false
	}
	defer rows.Close()
	for rows.Next() {
		var cid, notnull, pk int
		var name, ctype string
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return false
		}
		if name == col {
			return true
		}
	}
	return false
}

// nullStr returns the string value or "" for a NULL.
func nullStr(s sql.NullString) string {
	if s.Valid {
		return s.String
	}
	return ""
}

// nullF64 returns the float value or 0 for a NULL.
func nullF64(f sql.NullFloat64) float64 {
	if f.Valid {
		return f.Float64
	}
	return 0
}

// nullI64 returns the int value or 0 for a NULL.
func nullI64(i sql.NullInt64) int64 {
	if i.Valid {
		return i.Int64
	}
	return 0
}

// pdOpenStore opens the local mirror for the read-only novel commands:
// read-only when the schema exists (no write lock -> no SQLITE_BUSY under
// parallel reads), else read-write-migrate, with a short retry to ride out the
// first-run create/migrate race. Resolves the path via novelDBPath.
func pdOpenStore(ctx context.Context, dbPath string) (*store.Store, error) {
	path := novelDBPath(dbPath)
	var lastErr error
	for attempt := 0; attempt < 8; attempt++ {
		if st, ok := pdTryReadOnlyMigrated(path); ok {
			return st, nil
		}
		st, err := store.OpenWithContext(ctx, path)
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

func pdTryReadOnlyMigrated(path string) (*store.Store, bool) {
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
