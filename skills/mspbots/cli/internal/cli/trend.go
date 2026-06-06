// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

// pp:data-source local

package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// trendPoint is one snapshot's aggregated value for the chosen column.
type trendPoint struct {
	SnapshotID int64    `json:"snapshot_id"`
	TakenAt    string   `json:"taken_at"`
	RowCount   int      `json:"row_count"`
	Value      float64  `json:"value"`
	NonNumeric int      `json:"non_numeric_rows,omitempty"`
	Delta      *float64 `json:"delta,omitempty"`
}

type trendView struct {
	Alias  string       `json:"alias"`
	Column string       `json:"column"`
	Agg    string       `json:"agg"`
	Points []trendPoint `json:"points"`
	Note   string       `json:"note,omitempty"`
}

func newNovelTrendCmd(flags *rootFlags) *cobra.Command {
	var flagColumn string
	var agg string
	var limit int
	var dbPath string

	cmd := &cobra.Command{
		Use:   "trend [alias]",
		Short: "Time-series and point-over-point deltas for a numeric column across stored snapshots",
		Long: strings.Trim(`
Use this command for the time-series movement of a numeric column across
snapshots. Do NOT use it for row-level added/removed differences; use 'diff' instead.

For each stored snapshot of the alias, the chosen column is aggregated over
the rows (--agg sum|avg|count|min|max; 'count' counts rows and needs no
--column) and emitted oldest-first with a delta against the previous point.
Number-shaped strings ("42.5") count as numbers — the BI-export norm.
Capture snapshots first with 'snapshot'.`, "\n"),
		Example: strings.Trim(`
  mspbots-cli trend open-tickets --column "Open Count" --json
  mspbots-cli trend open-tickets --agg count --json`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only": "true",
			"pp:happy-args": "alias=example-alias;--agg=count",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if len(args) < 1 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("trend needs <alias>"))
			}
			if dryRunOK(flags) {
				fmt.Fprintf(cmd.OutOrStdout(), "would compute %s(%s) across snapshots of %q\n", agg, flagColumn, args[0])
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return usageErr(err)
			}
			switch agg {
			case "sum", "avg", "min", "max":
				if flagColumn == "" {
					_ = cmd.Usage()
					return usageErr(fmt.Errorf("--column is required for --agg %s (use --agg count to count rows)", agg))
				}
			case "count":
				// row count; no column needed
			default:
				return usageErr(fmt.Errorf("--agg must be sum, avg, count, min, or max; got %q", agg))
			}
			db, err := openMspbotsStore(cmd.Context(), dbPath)
			if err != nil {
				return err
			}
			defer db.Close()
			alias := args[0]
			snaps, err := db.SnapshotsForAlias(cmd.Context(), alias, limit)
			if err != nil {
				return fmt.Errorf("listing snapshots: %w", err)
			}
			if len(snaps) == 0 {
				return notFoundErr(fmt.Errorf("alias %q has no snapshots; run 'mspbots-cli snapshot %s' first", alias, alias))
			}
			// SnapshotsForAlias returns newest-first; series read oldest-first.
			points := make([]trendPoint, 0, len(snaps))
			for i := len(snaps) - 1; i >= 0; i-- {
				meta := snaps[i]
				_, rows, err := db.SnapshotRows(cmd.Context(), meta.ID)
				if err != nil {
					return fmt.Errorf("loading snapshot %d: %w", meta.ID, err)
				}
				p := trendPoint{SnapshotID: meta.ID, TakenAt: meta.TakenAt, RowCount: meta.RowCount}
				if agg == "count" {
					p.Value = float64(len(rows))
				} else {
					var nums []float64
					for _, row := range rows {
						if f, ok := numericValue(row, flagColumn); ok {
							nums = append(nums, f)
						} else {
							p.NonNumeric++
						}
					}
					p.Value = aggregate(agg, nums)
				}
				points = append(points, p)
			}
			for i := 1; i < len(points); i++ {
				d := points[i].Value - points[i-1].Value
				points[i].Delta = &d
			}
			view := trendView{Alias: alias, Column: flagColumn, Agg: agg, Points: points}
			if len(points) == 1 {
				view.Note = "only one snapshot exists; deltas appear once a second snapshot is captured"
			}
			return flags.printJSON(cmd, view)
		},
	}
	cmd.Flags().StringVar(&flagColumn, "column", "", `Numeric column to aggregate (exact name, spaces allowed: --column "Open Count")`)
	cmd.Flags().StringVar(&agg, "agg", "sum", "Aggregation across each snapshot's rows: sum, avg, count, min, or max")
	cmd.Flags().IntVar(&limit, "limit", 50, "Maximum snapshots in the series (newest N, rendered oldest-first)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (defaults to the CLI's local store)")
	return cmd
}

func aggregate(agg string, nums []float64) float64 {
	if len(nums) == 0 {
		return 0
	}
	switch agg {
	case "sum":
		var t float64
		for _, n := range nums {
			t += n
		}
		return t
	case "avg":
		var t float64
		for _, n := range nums {
			t += n
		}
		return t / float64(len(nums))
	case "min":
		m := nums[0]
		for _, n := range nums[1:] {
			if n < m {
				m = n
			}
		}
		return m
	case "max":
		m := nums[0]
		for _, n := range nums[1:] {
			if n > m {
				m = n
			}
		}
		return m
	default:
		return 0
	}
}
