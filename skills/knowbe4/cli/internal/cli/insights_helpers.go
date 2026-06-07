// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored helpers for the transcendence (insights) commands.

package cli

import (
	"database/sql"
	"fmt"

	"github.com/spf13/cobra"

	"knowbe4-pp-cli/internal/insights"
	"knowbe4-pp-cli/internal/store"
)

// openInsightsDB opens the local SQLite store (creating the insights-owned tables
// if needed) for the transcendence commands. It returns both the raw *sql.DB the
// insights queries consume and the *store.Store the framework sync-hint helpers
// need. The caller must invoke the returned close function. dbPath empty falls
// back to the standard location.
func openInsightsDB(cmd *cobra.Command, dbPath string) (*sql.DB, *store.Store, func(), error) {
	if dbPath == "" {
		dbPath = defaultDBPath("knowbe4-cli")
	}
	st, err := store.OpenWithContext(cmd.Context(), dbPath)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("opening local store: %w (run `knowbe4-cli sync` first)", err)
	}
	db := st.DB()
	if err := insights.EnsureSchema(cmd.Context(), db); err != nil {
		_ = st.Close()
		return nil, nil, nil, err
	}
	return db, st, func() { _ = st.Close() }, nil
}

// insightsSyncHints emits the framework unsynced/stale stderr hints for the
// named resource ("" scans all local resources) before a transcendence command
// returns local results. Hints write only to stderr, so JSON output is stable.
func insightsSyncHints(cmd *cobra.Command, st *store.Store, resourceType string, flags *rootFlags) {
	if !hintIfUnsynced(cmd, st, resourceType) {
		hintIfStale(cmd, st, resourceType, flags.maxAge)
	}
}
