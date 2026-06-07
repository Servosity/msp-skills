// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// staleQuote is one aging quote with the line-item value still at risk.
type staleQuote struct {
	ID          string  `json:"id"`
	Number      string  `json:"number"`
	Title       string  `json:"title"`
	Company     string  `json:"company"`
	Status      string  `json:"status"`
	AgeDays     int     `json:"ageDays"`
	AgeFrom     string  `json:"ageFrom"`     // which timestamp the age is measured from
	AtRiskValue float64 `json:"atRiskValue"` // sum(price*quantity) of line items
}

type staleQuoteReport struct {
	Quotes      []staleQuote `json:"quotes"`
	TotalAtRisk float64      `json:"totalAtRisk"`
	Note        string       `json:"note,omitempty"`
}

// newNovelQuoteStaleCmd implements the "quote stale" transcendence command:
// quotes sent or approved but aging past a cutoff, with the at-risk value.
// pp:data-source local
func newNovelQuoteStaleCmd(flags *rootFlags) *cobra.Command {
	var flagDays int
	var dbPath string

	cmd := &cobra.Command{
		Use:   "stale",
		Short: "Surface quotes sent or approved but aging past a cutoff, with the dollar value still at risk",
		Long: "Use this command to find quotes aging in an open lifecycle state (sent/approved) and the total\n" +
			"revenue at risk. Do NOT use this command to find under-priced line items inside quotes; use\n" +
			"'quote thin' instead.\n\n" +
			"Scans the local quote store for open quotes (sent or approved but not declined/expired) whose\n" +
			"age — measured from approvedAt, else sentAt, else createdAt — exceeds --days. At-risk value is\n" +
			"the sum of line-item price x quantity. No single API call answers this; run `sync` first.",
		Example: "  salesbuildr-cli quote stale --days 14\n" +
			"  salesbuildr-cli quote stale --days 7 --json --select quotes,totalAtRisk\n" +
			"  salesbuildr-cli quote stale --agent",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			if flagDays < 0 {
				return usageErr(fmt.Errorf("--days must be >= 0"))
			}
			quotes, err := loadQuotesHinted(cmd, flags, dbPath)
			if err != nil {
				return err
			}

			now := time.Now().UTC()
			report := staleQuoteReport{Quotes: make([]staleQuote, 0)}
			for _, q := range quotes {
				if !q.isOpen() {
					continue
				}
				stage := q.lifecycleStage()
				if stage != "sent" && stage != "approved" {
					continue
				}
				ref, refField := q.ageReference()
				if ref.IsZero() {
					continue
				}
				age := int(now.Sub(ref).Hours() / 24)
				if age < flagDays {
					continue
				}
				report.Quotes = append(report.Quotes, staleQuote{
					ID:          q.ID,
					Number:      q.Number,
					Title:       q.Title,
					Company:     q.Company,
					Status:      firstNonEmpty(q.Status, stage),
					AgeDays:     age,
					AgeFrom:     refField,
					AtRiskValue: q.value(),
				})
			}
			sort.Slice(report.Quotes, func(i, j int) bool {
				if report.Quotes[i].AtRiskValue != report.Quotes[j].AtRiskValue {
					return report.Quotes[i].AtRiskValue > report.Quotes[j].AtRiskValue
				}
				return report.Quotes[i].AgeDays > report.Quotes[j].AgeDays
			})
			for _, s := range report.Quotes {
				report.TotalAtRisk += s.AtRiskValue
			}
			if len(quotes) == 0 {
				report.Note = "no quotes in local store — run `salesbuildr-cli sync` first"
			} else if len(report.Quotes) == 0 {
				report.Note = fmt.Sprintf("no open quotes older than %d days", flagDays)
			}

			w := cmd.OutOrStdout()
			if flags.asJSON || flags.agent || !isTerminal(w) {
				return printJSONFiltered(w, report, flags)
			}
			if len(report.Quotes) == 0 {
				fmt.Fprintln(w, report.Note)
				return nil
			}
			tw := newTabWriter(w)
			fmt.Fprintln(tw, "NUMBER\tCOMPANY\tSTATUS\tAGE(d)\tFROM\tAT-RISK")
			for _, s := range report.Quotes {
				fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%s\t%.2f\n",
					orDash(s.Number), truncate(s.Company, 28), s.Status, s.AgeDays, s.AgeFrom, s.AtRiskValue)
			}
			fmt.Fprintf(tw, "\t\t\t\tTOTAL\t%.2f\n", report.TotalAtRisk)
			return tw.Flush()
		},
	}
	cmd.Flags().IntVar(&flagDays, "days", 14, "Minimum age in days (from approvedAt, else sentAt, else createdAt)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (defaults to standard location)")
	return cmd
}

func orDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "-"
	}
	return s
}
