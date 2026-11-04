// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel command: leaderboard.
// pp:data-source local

package cli

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

type leaderboardRow struct {
	Rank             int     `json:"rank"`
	Tenant           string  `json:"tenant"`
	TenantID         string  `json:"tenant_id"`
	Score            float64 `json:"score"`
	MaximumScore     float64 `json:"maximum_score"`
	Percent          float64 `json:"percent"`
	MaturityLevel    string  `json:"maturity_level"`
	WeakestFunction  string  `json:"weakest_nist_function"`
	WeakestDeduction float64 `json:"weakest_deduction"`
	ratio            float64
}

func newNovelLeaderboardCmd(flags *rootFlags) *cobra.Command {
	var order, dbPath string
	var limit int

	cmd := &cobra.Command{
		Use:   "leaderboard",
		Short: "Rank tenants by security posture score as a share of their maximum",
		Long: `Ranks the posture ratings fleet-sync stored for every tenant by score divided
by maximumScore (the maximum varies by CompassOne edition, so raw scores are
not comparable). The weakest NIST function is the one with the largest total
deduction across applied, failed metric rows; rows with metricApplied false
deduct nothing and are ignored. Worst first by default.`,
		Example: strings.Trim(`
  blackpoint-cli leaderboard --agent
  blackpoint-cli leaderboard --order best --limit 5`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "leaderboard: rank local posture ratings across tenants")
			}
			if limit < 0 {
				return usageErr(fmt.Errorf("--limit must be zero or more"))
			}
			if order != "worst" && order != "best" {
				return usageErr(fmt.Errorf("--order must be worst or best, got %q", order))
			}
			db, ok, err := openFleetForRead(cmd, flags, &dbPath)
			if err != nil {
				return err
			}
			if !ok {
				return emitRows(cmd, flags, []leaderboardRow{})
			}
			defer db.Close()
			if err := checkFleetKind(cmd, flags, db.DB(), fleetKindPosture); err != nil {
				return err
			}
			idx, _, err := fleetTenantIndex(db.DB())
			if err != nil {
				return err
			}
			recs, err := loadCleanFleetRows(db.DB(), fleetKindPosture, fleetKindPosture)
			if err != nil {
				return err
			}
			rows := buildLeaderboardRows(recs, idx, order == "best")
			if limit > 0 && len(rows) > limit {
				rows = rows[:limit]
			}
			return emitRows(cmd, flags, rows, "rank", "tenant", "percent", "score", "maximum_score", "maturity_level", "weakest_nist_function")
		},
	}
	cmd.Flags().StringVar(&order, "order", "worst", "Sort order: worst (lowest percent first) or best")
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum rows to return (0 = all)")
	cmd.Flags().StringVar(&dbPath, "db", "", "SQLite database path (default: the CLI's local data.db)")
	return cmd
}

// weakestNISTFunction sums deductions of applied, failed metric rows by
// nistFunction and returns the largest.
func weakestNISTFunction(o map[string]any) (string, float64) {
	totals := map[string]float64{}
	for _, m := range fieldList(o, "metricCalculationResults") {
		if applied, ok := fieldBool(m, "metricApplied"); ok && !applied {
			continue
		}
		if passed, ok := fieldBool(m, "metricPassed"); ok && passed {
			continue
		}
		fn := fieldStr(m, "nistFunction")
		if fn == "" {
			continue
		}
		d, _ := fieldNum(m, "deduction")
		totals[fn] += d
	}
	best, bestD := "", 0.0
	for fn, d := range totals {
		if d > bestD || (d == bestD && fn < best) {
			best, bestD = fn, d
		}
	}
	return best, bestD
}

func buildLeaderboardRows(recs []fleetRecord, idx map[string]fleetTenant, bestFirst bool) []leaderboardRow {
	rows := []leaderboardRow{}
	for _, r := range recs {
		o := r.Obj
		// Without both numbers there is no comparable percentage; skip rather
		// than rank the tenant as 0%.
		score, ok := fieldNum(o, "score")
		maxScore, hasMax := fieldNum(o, "maximumScore")
		if !ok || !hasMax || maxScore <= 0 {
			continue
		}
		ratio := score / maxScore
		pct := math.Round(ratio*1000) / 10
		fn, d := weakestNISTFunction(o)
		rows = append(rows, leaderboardRow{
			Tenant: tenantName(idx, r.Scope), TenantID: r.Scope, Score: score, MaximumScore: maxScore,
			Percent: pct, MaturityLevel: fieldStr(o, "maturityLevel"), WeakestFunction: fn, WeakestDeduction: d, ratio: ratio,
		})
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].ratio == rows[j].ratio {
			return rows[i].Tenant < rows[j].Tenant
		}
		if bestFirst {
			return rows[i].ratio > rows[j].ratio
		}
		return rows[i].ratio < rows[j].ratio
	})
	for i := range rows {
		rows[i].Rank = i + 1
	}
	return rows
}
