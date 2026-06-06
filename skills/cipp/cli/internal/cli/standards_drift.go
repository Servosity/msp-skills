// Copyright 2026 damienstevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"sort"
	"strings"

	"cipp-pp-cli/internal/store"
	"github.com/spf13/cobra"
)

// standardsSnapshotRow is one stored standards row with the snapshot timestamp
// it was synced at.
type standardsSnapshotRow struct {
	tenant   string
	standard string
	state    string
	syncedAt string
}

// driftRow reports one tenant's standard whose state changed between the two
// most recent snapshots.
type driftRow struct {
	Tenant        string `json:"tenant"`
	Standard      string `json:"standard"`
	PreviousState string `json:"previousState"`
	CurrentState  string `json:"currentState"`
}

// standardName resolves a stable identifier for a standards row.
func standardName(obj map[string]any) string {
	for _, key := range []string{"Standard", "standardName", "name", "Name", "standard", "templateName"} {
		if v := tenantFieldLookup(obj, key); v != "" {
			return v
		}
	}
	// Fall back to a stored id if present.
	if v := tenantFieldLookup(obj, "id", "GUID", "guid"); v != "" {
		return v
	}
	return ""
}

// standardState resolves the state/value of a standards row that drift is
// measured against.
func standardState(obj map[string]any) string {
	for _, key := range []string{"State", "state", "Status", "status", "Value", "value", "Compliance", "compliant"} {
		for k, v := range obj {
			if strings.EqualFold(k, key) && v != nil {
				if s, ok := v.(string); ok {
					return s
				}
				return fmt.Sprintf("%v", v)
			}
		}
	}
	return ""
}

// loadStandardsSnapshots reads the append-only standards_snapshots history
// written by `fanout --endpoint /ListStandards --save` (see
// standards_history.go). The generic resources table cannot back drift: it
// upserts on (resource_type, id), so each new save overwrites the prior row
// and at most one snapshot per standard ever exists there.
func loadStandardsSnapshots(db *store.Store) ([]standardsSnapshotRow, error) {
	// Absent history table = no snapshots captured yet.
	var tables int
	if err := db.DB().QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'standards_snapshots'`,
	).Scan(&tables); err != nil {
		return nil, err
	}
	if tables == 0 {
		return nil, nil
	}
	rows, err := db.Query(
		`SELECT tenant, standard, state, snapshot_ts FROM standards_snapshots ORDER BY snapshot_ts`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []standardsSnapshotRow
	for rows.Next() {
		var r standardsSnapshotRow
		if err := rows.Scan(&r.tenant, &r.standard, &r.state, &r.syncedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func newNovelStandardsDriftCmd(flags *rootFlags) *cobra.Command {
	var flagSince string
	var flagDB string

	cmd := &cobra.Command{
		Use:         "drift",
		Short:       "Show every tenant whose security baseline regressed since the last sync.",
		Annotations: map[string]string{"mcp:read-only": "true"},
		Long: `Compare the two most recent synced snapshots of standards per tenant and
report standards whose state changed. If only one snapshot exists for a tenant,
no drift is reported (drift is never fabricated from a single point in time).

Sync standards twice over time (e.g. via 'cipp-cli fanout --endpoint
/ListStandards --all-tenants --save') to build the snapshot history drift needs.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Read-only analytic with no required flag: a bare call still runs.
			if dryRunOK(flags) {
				return nil
			}
			_ = flagSince // accepted for forward-compat; comparison is snapshot-based

			dbPath := flagDB
			if dbPath == "" {
				dbPath = defaultDBPath("cipp-cli")
			}
			db, err := store.OpenWithContext(cmd.Context(), dbPath)
			if err != nil {
				return fmt.Errorf("opening local database: %w", err)
			}
			defer db.Close()

			snaps, err := loadStandardsSnapshots(db)
			if err != nil {
				return fmt.Errorf("reading standards snapshots: %w", err)
			}
			if len(snaps) == 0 {
				// Honest-empty: name the population path instead of silently
				// printing nothing.
				fmt.Fprintln(cmd.ErrOrStderr(),
					"no standards snapshot history; run 'cipp-cli fanout --endpoint /ListStandards --all-tenants --save' at two points in time to build it")
			}

			// Group by tenant → ordered distinct snapshot timestamps → per
			// standard state at each snapshot.
			type tenantData struct {
				snapshots []string                     // distinct synced_at, sorted ascending
				states    map[string]map[string]string // syncedAt -> standard -> state
			}
			byTenant := map[string]*tenantData{}
			for _, s := range snaps {
				td := byTenant[s.tenant]
				if td == nil {
					td = &tenantData{states: map[string]map[string]string{}}
					byTenant[s.tenant] = td
				}
				if td.states[s.syncedAt] == nil {
					td.states[s.syncedAt] = map[string]string{}
					td.snapshots = append(td.snapshots, s.syncedAt)
				}
				td.states[s.syncedAt][s.standard] = s.state
			}

			out := make([]driftRow, 0)
			tenants := make([]string, 0, len(byTenant))
			for t := range byTenant {
				tenants = append(tenants, t)
			}
			sort.Strings(tenants)

			for _, t := range tenants {
				td := byTenant[t]
				sort.Strings(td.snapshots)
				// Need at least two distinct snapshots to detect drift.
				if len(td.snapshots) < 2 {
					continue
				}
				prevTS := td.snapshots[len(td.snapshots)-2]
				currTS := td.snapshots[len(td.snapshots)-1]
				prev := td.states[prevTS]
				curr := td.states[currTS]

				standards := map[string]bool{}
				for s := range prev {
					standards[s] = true
				}
				for s := range curr {
					standards[s] = true
				}
				names := make([]string, 0, len(standards))
				for s := range standards {
					names = append(names, s)
				}
				sort.Strings(names)
				for _, name := range names {
					pv := prev[name]
					cv := curr[name]
					if pv != cv {
						out = append(out, driftRow{
							Tenant:        t,
							Standard:      name,
							PreviousState: pv,
							CurrentState:  cv,
						})
					}
				}
			}

			if flags.asJSON {
				return flags.printJSON(cmd, out)
			}
			headers := []string{"TENANT", "STANDARD", "PREVIOUS", "CURRENT"}
			tableRows := make([][]string, 0, len(out))
			for _, r := range out {
				tableRows = append(tableRows, []string{r.Tenant, r.Standard, r.PreviousState, r.CurrentState})
			}
			return flags.printTable(cmd, headers, tableRows)
		},
	}
	cmd.Flags().StringVar(&flagSince, "since", "", "Reserved: time window hint (e.g. 7d); drift is computed from the two most recent snapshots")
	cmd.Flags().StringVar(&flagDB, "db", "", "Path to the local SQLite database (default: standard location)")
	return cmd
}
