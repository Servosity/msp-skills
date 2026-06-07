// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"sort"
	"time"

	"action1-pp-cli/internal/store"
	"github.com/spf13/cobra"
)

type driftChange struct {
	OrganizationID string `json:"organization_id"`
	EndpointID     string `json:"endpoint_id"`
	Name           string `json:"name"`
	PreviousMiss   int    `json:"previous_missing"`
	CurrentMiss    int    `json:"current_missing"`
	Delta          int    `json:"delta"`
	Status         string `json:"status"`
}

type driftSummary struct {
	BaselineTakenAt string `json:"baseline_taken_at"`
	CurrentTakenAt  string `json:"current_taken_at"`
	Endpoints       int    `json:"endpoints_compared"`
	Remediated      int    `json:"remediated"`
	Regressed       int    `json:"regressed"`
	Cleared         int    `json:"cleared"`
	NewlyMissing    int    `json:"newly_missing"`
	Note            string `json:"note,omitempty"`
}

type driftResult struct {
	Summary driftSummary  `json:"summary"`
	Changes []driftChange `json:"changes"`
}

// pp:data-source local
func newNovelFleetPatchDriftCmd(flags *rootFlags) *cobra.Command {
	var dbPath, orgFilter string
	var limit int
	var noCapture, includeUnchanged bool

	cmd := &cobra.Command{
		Use:         "patch-drift",
		Short:       "Diff patch posture over time: what was remediated and what newly appeared.",
		Annotations: map[string]string{"mcp:read-only": "true"},
		Long: `Patch drift across the fleet over time. The Action1 API is stateless — it
cannot tell you what changed. This command keeps its own per-endpoint
missing-update snapshots in the local store: each run records the current
posture, then diffs it against the previous snapshot to show remediated,
regressed, cleared, and newly-missing endpoints.

Run it after each 'sync' to build the time series. The first run records a
baseline; the second and later runs show drift. Use --no-capture to diff the
two most recent existing snapshots without recording a new one.`,
		Example: `  action1-cli sync --full && action1-cli fleet patch-drift --agent
  action1-cli fleet patch-drift --no-capture --org <org-uuid>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			db, err := fleetOpenStore(cmd.Context(), dbPath)
			if err != nil {
				return err
			}
			defer db.Close()

			if err := driftEnsureTable(cmd.Context(), db); err != nil {
				return err
			}

			if !noCapture {
				maybeEmitSyncHints(cmd, db, "endpoints", flags.maxAge)
				endpoints, err := fleetLoadAll(cmd.Context(), db, "endpoints")
				if err != nil {
					return err
				}
				if err := driftCapture(cmd.Context(), db, endpoints); err != nil {
					return err
				}
			}

			snaps, err := driftRecentSnapshots(cmd.Context(), db)
			if err != nil {
				return err
			}

			result := driftResult{Changes: make([]driftChange, 0)}
			if len(snaps) < 2 {
				result.Summary.Note = "baseline captured — run again after the next sync to see drift"
				if len(snaps) == 1 {
					result.Summary.CurrentTakenAt = snaps[0]
				}
				return fleetEmit(cmd, flags, result, []string{"INFO"}, [][]string{{result.Summary.Note}})
			}

			current, baseline := snaps[0], snaps[1]
			curMap, err := driftLoadSnapshot(cmd.Context(), db, current, orgFilter)
			if err != nil {
				return err
			}
			baseMap, err := driftLoadSnapshot(cmd.Context(), db, baseline, orgFilter)
			if err != nil {
				return err
			}

			seen := map[string]bool{}
			ids := make([]string, 0, len(curMap)+len(baseMap))
			for id := range curMap {
				if !seen[id] {
					seen[id] = true
					ids = append(ids, id)
				}
			}
			for id := range baseMap {
				if !seen[id] {
					seen[id] = true
					ids = append(ids, id)
				}
			}

			result.Summary.BaselineTakenAt = baseline
			result.Summary.CurrentTakenAt = current
			for _, id := range ids {
				cur, hasCur := curMap[id]
				base, hasBase := baseMap[id]
				delta := cur.missing - base.missing
				status := "unchanged"
				switch {
				case !hasBase && cur.missing > 0:
					status = "newly_missing"
					result.Summary.NewlyMissing++
				case hasCur && hasBase && base.missing > 0 && cur.missing == 0:
					status = "cleared"
					result.Summary.Cleared++
				case delta < 0:
					status = "remediated"
					result.Summary.Remediated++
				case delta > 0:
					status = "regressed"
					result.Summary.Regressed++
				}
				result.Summary.Endpoints++
				if status == "unchanged" && !includeUnchanged {
					continue
				}
				row := cur
				if !hasCur {
					row = base
				}
				result.Changes = append(result.Changes, driftChange{
					OrganizationID: row.org,
					EndpointID:     id,
					Name:           row.name,
					PreviousMiss:   base.missing,
					CurrentMiss:    cur.missing,
					Delta:          delta,
					Status:         status,
				})
			}

			sort.SliceStable(result.Changes, func(i, j int) bool {
				return result.Changes[i].Delta < result.Changes[j].Delta // biggest remediations first
			})
			if limit > 0 && len(result.Changes) > limit {
				result.Changes = result.Changes[:limit]
			}

			header := []string{"ORG", "ENDPOINT", "NAME", "PREV", "CURR", "DELTA", "STATUS"}
			matrix := make([][]string, 0, len(result.Changes))
			for _, c := range result.Changes {
				matrix = append(matrix, []string{c.OrganizationID, c.EndpointID, c.Name,
					fleetItoa(float64(c.PreviousMiss)), fleetItoa(float64(c.CurrentMiss)),
					fleetItoa(float64(c.Delta)), c.Status})
			}
			return fleetEmit(cmd, flags, result, header, matrix)
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Local database path (default: standard cache location)")
	cmd.Flags().StringVar(&orgFilter, "org", fleetEnvOrg(), "Limit to one organization ID (default: $ACTION1_ORG_ID)")
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum changes to return (0 = all)")
	cmd.Flags().BoolVar(&noCapture, "no-capture", false, "Diff the two most recent snapshots without recording a new one")
	cmd.Flags().BoolVar(&includeUnchanged, "all", false, "Include endpoints whose posture did not change")
	return cmd
}

type driftPoint struct {
	org     string
	name    string
	missing int
}

func driftEnsureTable(ctx context.Context, db *store.Store) error {
	_, err := db.DB().ExecContext(ctx, `CREATE TABLE IF NOT EXISTS fleet_patch_snapshots (
		taken_at TEXT NOT NULL,
		endpoint_id TEXT NOT NULL,
		org_id TEXT,
		name TEXT,
		missing INTEGER NOT NULL,
		PRIMARY KEY (taken_at, endpoint_id)
	)`)
	return err
}

func driftCapture(ctx context.Context, db *store.Store, endpoints []map[string]any) error {
	taken := time.Now().UTC().Format(time.RFC3339Nano)
	tx, err := db.DB().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.PrepareContext(ctx, `INSERT OR REPLACE INTO fleet_patch_snapshots
		(taken_at, endpoint_id, org_id, name, missing) VALUES (?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, e := range endpoints {
		id := fleetStrField(e, "id")
		if id == "" {
			continue
		}
		mu := fleetNested(e, "missing_updates")
		missing := 0
		if mu != nil {
			c, _ := fleetNum(mu["critical"])
			o, _ := fleetNum(mu["other"])
			missing = int(c) + int(o)
		}
		name := fleetStrField(e, "name")
		if name == "" {
			name = fleetStrField(e, "device_name")
		}
		if _, err := stmt.ExecContext(ctx, taken, id, fleetOrgID(e), name, missing); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func driftRecentSnapshots(ctx context.Context, db *store.Store) ([]string, error) {
	rows, err := db.DB().QueryContext(ctx,
		`SELECT taken_at FROM fleet_patch_snapshots GROUP BY taken_at ORDER BY taken_at DESC LIMIT 2`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func driftLoadSnapshot(ctx context.Context, db *store.Store, takenAt, orgFilter string) (map[string]driftPoint, error) {
	q := `SELECT endpoint_id, org_id, name, missing FROM fleet_patch_snapshots WHERE taken_at = ?`
	args := []any{takenAt}
	if orgFilter != "" {
		q += ` AND org_id = ?`
		args = append(args, orgFilter)
	}
	rows, err := db.DB().QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]driftPoint{}
	for rows.Next() {
		var id, org, name string
		var missing int
		if err := rows.Scan(&id, &org, &name, &missing); err != nil {
			return nil, err
		}
		out[id] = driftPoint{org: org, name: name, missing: missing}
	}
	return out, rows.Err()
}
