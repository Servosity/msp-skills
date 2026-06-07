// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"
)

type valueGroup struct {
	Key       string  `json:"key"`
	Documents int     `json:"documents"`
	Value     float64 `json:"value"`
}

type valueReport struct {
	Status     string       `json:"status_filter"`
	GroupBy    string       `json:"group_by"`
	Documents  int          `json:"documents"`
	TotalValue float64      `json:"total_value"`
	Currency   string       `json:"currency,omitempty"`
	Groups     []valueGroup `json:"groups"`
	Note       string       `json:"note,omitempty"`
}

// newNovelValueCmd implements the "value" transcendence command: sums
// grand_total across open (or status-filtered) documents.
// pp:data-source local
func newNovelValueCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	var statusCSV string
	var groupBy string
	cmd := &cobra.Command{
		Use:         "value",
		Short:       "Sum the quote/pricing totals across all open (non-completed) documents — your in-flight dollar value.",
		Long:        "Sum open quote/pricing value across all in-flight documents, grouped by status or template.\n\nUse this command for a flat open-value TOTAL by status. Do NOT use it to bucket dollars by deal age/risk; use 'forecast' instead.\n\nReads the local store — run `sync` first. No API endpoint aggregates value across documents.",
		Example:     "  pandadoc-cli value --status sent,viewed --by template --agent",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			if groupBy != "status" && groupBy != "template" {
				return fmt.Errorf("invalid --by %q: expected 'status' or 'template'", groupBy)
			}
			if dbPath == "" {
				dbPath = defaultDBPath("pandadoc-cli")
			}
			docs, err := loadDocumentsHinted(cmd, flags, dbPath)
			if err != nil {
				return err
			}

			want := parseStatusList(statusCSV)
			report := valueReport{Status: statusCSV, GroupBy: groupBy, Groups: make([]valueGroup, 0)}
			groups := map[string]*valueGroup{}
			for _, d := range docs {
				if len(want) > 0 {
					if !want[normalizeStatus(d.Status)] {
						continue
					}
				} else if isTerminalStatus(d.Status) {
					// default scope is open documents
					continue
				}
				report.Documents++
				report.TotalValue += d.GrandTotal
				if d.Currency != "" && report.Currency == "" {
					report.Currency = d.Currency
				}
				var key string
				if groupBy == "template" {
					key = d.TemplateName
					if key == "" {
						key = "(no template)"
					}
				} else {
					key = shortStatus(d.Status)
				}
				g := groups[key]
				if g == nil {
					g = &valueGroup{Key: key}
					groups[key] = g
				}
				g.Documents++
				g.Value += d.GrandTotal
			}
			for _, g := range groups {
				report.Groups = append(report.Groups, *g)
			}
			sort.Slice(report.Groups, func(i, j int) bool {
				if report.Groups[i].Value != report.Groups[j].Value {
					return report.Groups[i].Value > report.Groups[j].Value
				}
				return report.Groups[i].Key < report.Groups[j].Key
			})
			if len(docs) == 0 {
				report.Note = "no documents in local store — run `pandadoc-cli sync` first"
			} else if report.TotalValue == 0 {
				report.Note = "matched documents have no grand_total in synced data; value totals are 0"
			}

			w := cmd.OutOrStdout()
			if flags.asJSON || flags.agent || !isTerminal(w) {
				return printJSONFiltered(w, report, flags)
			}
			cur := report.Currency
			if cur != "" {
				cur = " " + cur
			}
			fmt.Fprintf(w, "Open value: %.2f%s across %d documents\n\n", report.TotalValue, cur, report.Documents)
			for _, g := range report.Groups {
				fmt.Fprintf(w, "  %-24s %.2f  (%d docs)\n", g.Key, g.Value, g.Documents)
			}
			if report.Note != "" {
				fmt.Fprintf(w, "\n  %s\n", report.Note)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (defaults to standard location)")
	cmd.Flags().StringVar(&statusCSV, "status", "", "Comma-separated statuses to include (default: all open/non-terminal)")
	cmd.Flags().StringVar(&groupBy, "by", "status", "Group the breakdown by 'status' or 'template'")
	return cmd
}
