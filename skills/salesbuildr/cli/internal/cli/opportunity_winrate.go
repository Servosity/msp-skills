// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"
)

// winrateRow is the won/lost breakdown for one group (owner, stage, or category).
type winrateRow struct {
	Group      string  `json:"group"`
	Won        int     `json:"won"`
	Lost       int     `json:"lost"`
	Open       int     `json:"open"`
	WinRatePct float64 `json:"winRatePct"` // won / (won + lost)
}

type winrateReport struct {
	By   string       `json:"by"`
	Rows []winrateRow `json:"rows"`
	Won  int          `json:"won"`
	Lost int          `json:"lost"`
	Open int          `json:"open"`
	Note string       `json:"note,omitempty"`
}

// newNovelOpportunityWinrateCmd implements the "opportunity winrate"
// transcendence command: won/lost ratios grouped by owner, stage, or category.
// pp:data-source local
func newNovelOpportunityWinrateCmd(flags *rootFlags) *cobra.Command {
	var flagBy string
	var dbPath string

	cmd := &cobra.Command{
		Use:   "winrate",
		Short: "Compute won/lost ratios grouped by owner, stage, or category",
		Long: "Use this command for won/lost conversion ratios grouped by owner, stage, or category. Do NOT\n" +
			"use this command for cycle-time or stage-duration metrics; use 'opportunity velocity' instead.\n\n" +
			"Classifies every synced opportunity as won, lost, or open from its status, groups by the --by\n" +
			"dimension, and reports per-group win rates. The API has no aggregate endpoint. Run `sync` first.",
		Example: "  salesbuildr-cli opportunity winrate --by owner\n" +
			"  salesbuildr-cli opportunity winrate --by stage --agent",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			groupKey := func(o novelOpportunity) string { return firstNonEmpty(o.Owner, "(no owner)") }
			switch flagBy {
			case "owner":
			case "stage":
				groupKey = func(o novelOpportunity) string { return firstNonEmpty(o.Stage, "(no stage)") }
			case "category":
				groupKey = func(o novelOpportunity) string { return firstNonEmpty(o.Category, "(no category)") }
			default:
				return usageErr(fmt.Errorf("--by must be one of owner, stage, category (got %q)", flagBy))
			}
			opps, err := loadOpportunitiesHinted(cmd, flags, dbPath)
			if err != nil {
				return err
			}

			rows := map[string]*winrateRow{}
			report := winrateReport{By: flagBy, Rows: make([]winrateRow, 0)}
			for _, o := range opps {
				key := groupKey(o)
				r := rows[key]
				if r == nil {
					r = &winrateRow{Group: key}
					rows[key] = r
				}
				switch {
				case o.isWon():
					r.Won++
					report.Won++
				case o.isLost():
					r.Lost++
					report.Lost++
				default:
					r.Open++
					report.Open++
				}
			}
			for _, r := range rows {
				if denom := r.Won + r.Lost; denom > 0 {
					r.WinRatePct = float64(r.Won) / float64(denom) * 100
				}
				report.Rows = append(report.Rows, *r)
			}
			sort.Slice(report.Rows, func(i, j int) bool {
				ti := report.Rows[i].Won + report.Rows[i].Lost + report.Rows[i].Open
				tj := report.Rows[j].Won + report.Rows[j].Lost + report.Rows[j].Open
				return ti > tj
			})
			if len(opps) == 0 {
				report.Note = "no opportunities in local store — run `salesbuildr-cli sync` first"
			} else if report.Won+report.Lost == 0 {
				report.Note = "no closed (won/lost) opportunities yet — win rates need closed deals"
			}

			w := cmd.OutOrStdout()
			if flags.asJSON || flags.agent || !isTerminal(w) {
				return printJSONFiltered(w, report, flags)
			}
			if len(opps) == 0 {
				fmt.Fprintln(w, report.Note)
				return nil
			}
			tw := newTabWriter(w)
			fmt.Fprintln(tw, "GROUP\tWON\tLOST\tOPEN\tWIN%")
			for _, r := range report.Rows {
				fmt.Fprintf(tw, "%s\t%d\t%d\t%d\t%.1f\n", truncate(r.Group, 30), r.Won, r.Lost, r.Open, r.WinRatePct)
			}
			return tw.Flush()
		},
	}
	cmd.Flags().StringVar(&flagBy, "by", "owner", "Group dimension: owner, stage, or category")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (defaults to standard location)")
	return cmd
}
