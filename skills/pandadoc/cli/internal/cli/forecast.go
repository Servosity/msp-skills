// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

type forecastTier struct {
	Tier      string  `json:"tier"`
	Documents int     `json:"documents"`
	Value     float64 `json:"value"`
}

type forecastReport struct {
	Tiers       []forecastTier `json:"tiers"`
	OpenDocs    int            `json:"open_docs"`
	OpenValue   float64        `json:"open_value"`
	Currency    string         `json:"currency,omitempty"`
	NoValueDocs int            `json:"no_value_docs"`
	HealthyDays int            `json:"healthy_days"`
	AgingDays   int            `json:"aging_days"`
	Note        string         `json:"note,omitempty"`
}

// newNovelForecastCmd implements the "forecast" transcendence command:
// buckets open quote dollars into healthy / aging / stalled tiers by deal age.
// Joins quote totals with status-age buckets locally — the API has no value
// rollup at all, let alone a risk-tiered one.
// pp:data-source local
func newNovelForecastCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	var healthyDays int
	var agingDays int
	cmd := &cobra.Command{
		Use:         "forecast",
		Short:       "Bucket open quote dollars into healthy, aging, and stalled tiers by deal age.",
		Long:        "Bucket the open (non-terminal) document pipeline's quote dollars into healthy, aging, and stalled tiers by days since last activity.\n\nUse this command to bucket open quote DOLLARS by deal age/risk. Do NOT use it for a single flat open-value total; use 'value' instead. Do NOT use it to list the stalled documents themselves; use 'stalled'.\n\nReads the local store — run `sync` first. Tier boundaries default to <7 days healthy and 7-14 days aging; tune with --healthy-days/--aging-days.",
		Example:     "  pandadoc-cli forecast --agent",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			if healthyDays <= 0 || agingDays <= healthyDays {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--aging-days (%d) must be greater than --healthy-days (%d), both positive", agingDays, healthyDays))
			}
			docs, err := loadDocumentsHinted(cmd, flags, dbPath)
			if err != nil {
				return err
			}

			report := forecastReport{
				Tiers: []forecastTier{
					{Tier: "healthy"},
					{Tier: "aging"},
					{Tier: "stalled"},
				},
				HealthyDays: healthyDays,
				AgingDays:   agingDays,
			}
			for _, d := range docs {
				if isTerminalStatus(d.Status) {
					continue
				}
				report.OpenDocs++
				if d.GrandTotal == 0 {
					report.NoValueDocs++
				}
				if d.Currency != "" && report.Currency == "" {
					report.Currency = d.Currency
				}
				age := ageDays(d.lastActivity())
				idx := 0
				switch {
				case age < healthyDays:
					idx = 0
				case age <= agingDays:
					idx = 1
				default:
					idx = 2
				}
				report.Tiers[idx].Documents++
				report.Tiers[idx].Value += d.GrandTotal
				report.OpenValue += d.GrandTotal
			}
			if len(docs) == 0 {
				report.Note = "no documents in local store — run `pandadoc-cli sync` first"
			} else if report.OpenDocs == 0 {
				report.Note = "no open documents — the whole pipeline is in a terminal status"
			} else if report.NoValueDocs > 0 {
				report.Note = fmt.Sprintf("%d open document(s) carry no quote value and count toward documents but not dollars", report.NoValueDocs)
			}

			w := cmd.OutOrStdout()
			if flags.asJSON || flags.agent || !isTerminal(w) {
				return printJSONFiltered(w, report, flags)
			}
			cur := report.Currency
			if cur != "" {
				cur = " " + cur
			}
			fmt.Fprintf(w, "Pipeline value forecast — %d open docs, %.2f%s total\n\n", report.OpenDocs, report.OpenValue, cur)
			for _, t := range report.Tiers {
				span := ""
				switch t.Tier {
				case "healthy":
					span = fmt.Sprintf("< %dd", healthyDays)
				case "aging":
					span = fmt.Sprintf("%d-%dd", healthyDays, agingDays)
				case "stalled":
					span = fmt.Sprintf("> %dd", agingDays)
				}
				fmt.Fprintf(w, "  %-8s %-8s docs=%-4d value=%.2f%s\n", t.Tier, span, t.Documents, t.Value, cur)
			}
			if report.Note != "" {
				fmt.Fprintf(w, "\n  %s\n", report.Note)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (defaults to standard location)")
	cmd.Flags().IntVar(&healthyDays, "healthy-days", 7, "Deals younger than this many days are healthy")
	cmd.Flags().IntVar(&agingDays, "aging-days", 14, "Deals older than this many days are stalled; between healthy and this is aging")
	return cmd
}
