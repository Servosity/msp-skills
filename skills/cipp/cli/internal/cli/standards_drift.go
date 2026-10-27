// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source local

package cli

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"cipp-pp-cli/internal/cliutil"
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
		Short:       "Show every tenant whose Standards settings changed between two stored snapshots.",
		Example:     "  cipp-cli standards drift --since 7d --agent",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local"},
		Long: `Use this command for Standards settings that regressed between two stored snapshots.
Do NOT use this command for row-level changes in other resources; use 'changes' instead.

Compares, per tenant, the newest stored standards snapshot with the newest one
at or before --since ago (or with the previous snapshot when --since is not
given) and reports standards whose state changed. A tenant without two usable
snapshots reports no drift: drift is never fabricated from one point in time.
CIPP's own drift views report current deviation from a template; this compares
your own history.

Save standards at two points in time (e.g. 'cipp-cli fanout --endpoint
/ListStandards --all-tenants --save') to build the snapshot history drift needs.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Read-only analytic with no required flag: a bare call still runs.
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "standards drift")
			}
			var cutoff time.Time
			if flagSince != "" {
				d, err := cliutil.ParseDurationLoose(flagSince)
				if err != nil || d <= 0 {
					return usageErr(fmt.Errorf("invalid --since %q: use a positive duration such as 7d, 24h or 1w", flagSince))
				}
				cutoff = time.Now().UTC().Add(-d)
			}

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
					"hint: no standards snapshot history; run 'cipp-cli fanout --endpoint /ListStandards --all-tenants --save' at two points in time to build it")
			}

			out := computeStandardsDrift(snaps, cutoff)

			if flags.asJSON {
				return printNovelJSON(cmd, flags, out)
			}
			headers := []string{"TENANT", "STANDARD", "PREVIOUS", "CURRENT"}
			tableRows := make([][]string, 0, len(out))
			for _, r := range out {
				tableRows = append(tableRows, []string{r.Tenant, r.Standard, r.PreviousState, r.CurrentState})
			}
			return flags.printTable(cmd, headers, tableRows)
		},
	}
	cmd.Flags().StringVar(&flagSince, "since", "", "Compare against the newest snapshot at or before this long ago (e.g. 7d, 24h); default: the previous snapshot")
	cmd.Flags().StringVar(&flagDB, "db", "", "Path to the local SQLite database (default: standard location)")
	return cmd
}

// computeStandardsDrift diffs each tenant's base and current standards
// snapshot (see pickSnapshotPair) into per-standard state changes.
func computeStandardsDrift(snaps []standardsSnapshotRow, cutoff time.Time) []driftRow {
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
		sortSnapshotTimes(td.snapshots)
		// Need two usable snapshots to detect drift (see pickSnapshotPair).
		prevTS, currTS, ok := pickSnapshotPair(td.snapshots, cutoff)
		if !ok {
			continue
		}
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
	return out
}
