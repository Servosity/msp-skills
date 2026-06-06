// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"microsoft-graph-pp-cli/internal/store"
)

// graphDBPath resolves the local store path: the --db flag value if set,
// otherwise the canonical default.
func graphDBPath(dbFlag string) string {
	if dbFlag != "" {
		return dbFlag
	}
	return defaultDBPath("microsoft-graph-cli")
}

// loadDomainRows opens the local store read-only and returns the `data` JSON
// column for every row matched by query. A missing store file — or a store
// that exists but has not been synced (table absent) — yields an empty slice
// with no error, so the transcendence commands degrade to an honest empty
// result before the first `pull` rather than erroring. This is also what keeps
// them exit-0 under the verify harness, which runs them against no store.
func loadDomainRows(dbFlag, query string, args ...any) ([]json.RawMessage, error) {
	dbPath := graphDBPath(dbFlag)
	if _, err := os.Stat(dbPath); err != nil {
		return nil, nil
	}
	st, err := store.OpenReadOnly(dbPath)
	if err != nil {
		return nil, err
	}
	defer st.Close()
	rows, err := st.Query(query, args...)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	var out []json.RawMessage
	for rows.Next() {
		var data string
		if err := rows.Scan(&data); err != nil {
			return nil, err
		}
		out = append(out, json.RawMessage(data))
	}
	return out, rows.Err()
}

// parseWindow parses a relative time window like "24h", "90m", or "7d".
// time.ParseDuration handles h/m/s; the day suffix is expanded here because
// MSP triage and drift windows are naturally expressed in days. An empty
// string returns def.
func parseWindow(s string, def time.Duration) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return def, nil
	}
	if strings.HasSuffix(s, "d") {
		n, err := strconv.Atoi(strings.TrimSuffix(s, "d"))
		if err != nil || n < 0 {
			return 0, usageErr(fmt.Errorf("invalid window %q: expected a duration like 24h, 90m, or 7d", s))
		}
		return time.Duration(n) * 24 * time.Hour, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil || d < 0 {
		return 0, usageErr(fmt.Errorf("invalid window %q: expected a duration like 24h, 90m, or 7d", s))
	}
	return d, nil
}

// storeNeverSynced reports whether the local store has never been populated by
// a `pull`/`sync`. True when the db file is absent, unreadable, or its
// sync_state table has no rows. Used to distinguish "nothing synced yet" from
// "synced, genuinely nothing to report" so the transcendence commands never
// present a false-clean empty result on a fresh install.
func storeNeverSynced(dbFlag string) bool {
	dbPath := graphDBPath(dbFlag)
	if _, err := os.Stat(dbPath); err != nil {
		return true
	}
	st, err := store.OpenReadOnly(dbPath)
	if err != nil {
		return true
	}
	defer st.Close()
	var n int
	if err := st.DB().QueryRow(`SELECT COUNT(*) FROM sync_state`).Scan(&n); err != nil {
		return true
	}
	return n == 0
}

// hintUnsyncedIfEmpty writes a one-line stderr hint when a transcendence
// command produced an empty result only because the store was never synced.
// The hint goes to stderr so the stdout JSON envelope stays clean for agents;
// the command still exits 0 because an empty aggregation is a valid result
// shape, not an error.
func hintUnsyncedIfEmpty(cmd *cobra.Command, dbFlag string, isEmpty bool) {
	if !isEmpty {
		return
	}
	if storeNeverSynced(dbFlag) {
		fmt.Fprintln(cmd.ErrOrStderr(), "hint: local store has not been synced yet — run 'microsoft-graph-cli pull' first to populate it.")
	}
}
