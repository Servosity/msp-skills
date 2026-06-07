// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

// pp:data-source local
package cli

import (
	"database/sql"
	"sort"

	"github.com/spf13/cobra"
)

func newNovelClientsScorecardCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "scorecard",
		Short: "One posture row per client across all signals",
		Long: `Use this command for one posture row per client: agent count, online share,
failing checks, pending patches, and open alerts. Do NOT use it for the
whole-fleet single number; use 'fleet health' instead, or for patch counts
only; use 'patch posture' instead.

Joins the synced agents and alerts in the local store, grouped by client.`,
		Example:     "  tactical-rmm-cli clients scorecard\n  tactical-rmm-cli clients scorecard --json",
		Annotations: tRO(),
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			type row struct {
				Client         string  `json:"client"`
				AgentCount     int     `json:"agent_count"`
				OnlinePct      float64 `json:"online_pct"`
				FailingChecks  int     `json:"failing_checks"`
				PendingPatches int     `json:"pending_patches"`
				OpenAlerts     int     `json:"open_alerts"`
			}
			byClient := map[string]*row{}
			order := make([]string, 0)
			get := func(name string) *row {
				if name == "" {
					name = "(unknown)"
				}
				r, ok := byClient[name]
				if !ok {
					r = &row{Client: name}
					byClient[name] = r
					order = append(order, name)
				}
				return r
			}
			if s := novelLocalRead(cmd, flags, "agents"); s != nil {
				defer s.Close()
				ctx := cmd.Context()
				// Agent-derived signals grouped by client.
				aq := `SELECT
					COALESCE(json_extract(data,'$.client_name'),'(unknown)') c,
					COUNT(*),
					SUM(CASE WHEN json_extract(data,'$.status')='online' THEN 1 ELSE 0 END),
					COALESCE(SUM(json_extract(data,'$.checks.failing')),0),
					SUM(CASE WHEN json_extract(data,'$.has_patches_pending')=1 THEN 1 ELSE 0 END)
				FROM resources WHERE resource_type='agents' GROUP BY c`
				if rows, qe := s.DB().QueryContext(ctx, aq); qe == nil {
					for rows.Next() {
						var c sql.NullString
						var total, online, failing, patches sql.NullInt64
						if rows.Scan(&c, &total, &online, &failing, &patches) == nil {
							r := get(c.String)
							r.AgentCount = int(total.Int64)
							r.FailingChecks = int(failing.Int64)
							r.PendingPatches = int(patches.Int64)
							if total.Int64 > 0 {
								r.OnlinePct = float64(int((float64(online.Int64)/float64(total.Int64))*1000+0.5)) / 10
							}
						}
					}
					_ = rows.Close()
				}
				// Open alerts grouped by client.
				alq := `SELECT COALESCE(json_extract(data,'$.client'),'(unknown)') c, COUNT(*)
					FROM resources WHERE resource_type='alerts' AND COALESCE(json_extract(data,'$.resolved'),0)=0 GROUP BY c`
				if rows, qe := s.DB().QueryContext(ctx, alq); qe == nil {
					for rows.Next() {
						var c sql.NullString
						var n sql.NullInt64
						if rows.Scan(&c, &n) == nil {
							get(c.String).OpenAlerts = int(n.Int64)
						}
					}
					_ = rows.Close()
				}
			}
			out := make([]row, 0, len(order))
			for _, name := range order {
				out = append(out, *byClient[name])
			}
			sort.Slice(out, func(i, j int) bool {
				if out[i].FailingChecks == out[j].FailingChecks {
					return out[i].Client < out[j].Client
				}
				return out[i].FailingChecks > out[j].FailingChecks
			})
			return flags.printJSON(cmd, out)
		},
	}
	return cmd
}
