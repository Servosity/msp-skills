// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// funnelStage is one quote-lifecycle stage with its count and dollar value.
type funnelStage struct {
	Stage string  `json:"stage"`
	Count int     `json:"count"`
	Value float64 `json:"value"`
}

type funnelReport struct {
	Total             int           `json:"total"`
	Stages            []funnelStage `json:"stages"`
	SentToApprovedPct float64       `json:"sentToApprovedPct"` // approved / (approved + declined), 0 when no closed quotes
	Note              string        `json:"note,omitempty"`
}

// newNovelQuoteFunnelCmd implements the "quote funnel" transcendence command:
// stage-by-stage conversion of the quote lifecycle with value at each stage.
// pp:data-source local
func newNovelQuoteFunnelCmd(flags *rootFlags) *cobra.Command {
	var dbPath string

	cmd := &cobra.Command{
		Use:   "funnel",
		Short: "See stage-by-stage conversion of the quote lifecycle — count and dollar value at each stage",
		Long: "Use this command for quote-lifecycle stage conversion (sent->approved->declined) with value at\n" +
			"each stage. Do NOT use this command for opportunity-pipeline conversion; use 'opportunity\n" +
			"winrate' instead.\n\n" +
			"Buckets every quote in the local store into its furthest lifecycle stage (draft, sent,\n" +
			"approved, declined, expired) with count and line-item value rollups. The API returns quotes one\n" +
			"state at a time and never the funnel. Run `sync` first.",
		Example: "  salesbuildr-cli quote funnel\n" +
			"  salesbuildr-cli quote funnel --agent",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			quotes, err := loadQuotesHinted(cmd, flags, dbPath)
			if err != nil {
				return err
			}

			counts := map[string]int{}
			values := map[string]float64{}
			for _, q := range quotes {
				if !q.DeletedAt.IsZero() {
					continue
				}
				stage := q.lifecycleStage()
				counts[stage]++
				values[stage] += q.value()
			}

			report := funnelReport{Total: len(quotes), Stages: make([]funnelStage, 0, len(quoteLifecycleStages))}
			for _, stage := range quoteLifecycleStages {
				report.Stages = append(report.Stages, funnelStage{Stage: stage, Count: counts[stage], Value: values[stage]})
			}
			if denom := counts["approved"] + counts["declined"]; denom > 0 {
				report.SentToApprovedPct = float64(counts["approved"]) / float64(denom) * 100
			}
			if len(quotes) == 0 {
				report.Note = "no quotes in local store — run `salesbuildr-cli sync` first"
			}

			w := cmd.OutOrStdout()
			if flags.asJSON || flags.agent || !isTerminal(w) {
				return printJSONFiltered(w, report, flags)
			}
			if len(quotes) == 0 {
				fmt.Fprintln(w, report.Note)
				return nil
			}
			tw := newTabWriter(w)
			fmt.Fprintln(tw, "STAGE\tCOUNT\tVALUE")
			for _, s := range report.Stages {
				fmt.Fprintf(tw, "%s\t%d\t%.2f\n", s.Stage, s.Count, s.Value)
			}
			if err := tw.Flush(); err != nil {
				return err
			}
			fmt.Fprintf(w, "\napproved vs declined: %.1f%%\n", report.SentToApprovedPct)
			return nil
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (defaults to standard location)")
	return cmd
}
