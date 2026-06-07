// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"
)

type templateStat struct {
	TemplateID     string  `json:"template_id"`
	TemplateName   string  `json:"template_name"`
	Documents      int     `json:"documents"`
	Completed      int     `json:"completed"`
	Declined       int     `json:"declined"`
	Open           int     `json:"open"`
	CompletionRate float64 `json:"completion_rate"`
}

type templateStatsReport struct {
	Templates []templateStat `json:"templates"`
	Note      string         `json:"note,omitempty"`
}

func truncateField(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n <= 1 {
		return s[:n]
	}
	return s[:n-1] + "…"
}

// newNovelTemplateStatsCmd implements the "template-stats" transcendence
// command: per-template document counts and completion rates.
// pp:data-source local
func newNovelTemplateStatsCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	var limit int
	cmd := &cobra.Command{
		Use:         "template-stats",
		Short:       "Per-template document counts and completion rates — which templates actually close.",
		Long:        "Rank templates by how they perform: documents generated, completed, declined, still open, and a completion rate. Surfaces which templates actually close.\n\nReads the local store — run `sync` first. Joins documents back to their templates and aggregates terminal status, which the PandaDoc API has no endpoint for.",
		Example:     "  pandadoc-cli template-stats --agent",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			if dbPath == "" {
				dbPath = defaultDBPath("pandadoc-cli")
			}
			docs, err := loadDocumentsHinted(cmd, flags, dbPath)
			if err != nil {
				return err
			}

			type agg struct {
				name                          string
				docs, completed, declined, op int
			}
			byTmpl := map[string]*agg{}
			for _, d := range docs {
				key := d.TemplateID
				name := d.TemplateName
				if key == "" {
					key = "(no template)"
					if name == "" {
						name = "(no template)"
					}
				}
				a := byTmpl[key]
				if a == nil {
					a = &agg{name: name}
					byTmpl[key] = a
				}
				if a.name == "" {
					a.name = name
				}
				a.docs++
				switch normalizeStatus(d.Status) {
				case "document.completed", "document.paid":
					a.completed++
				case "document.declined", "document.voided", "document.rejected", "document.expired":
					a.declined++
				default:
					a.op++
				}
			}

			report := templateStatsReport{Templates: make([]templateStat, 0, len(byTmpl))}
			for id, a := range byTmpl {
				rate := 0.0
				if closed := a.completed + a.declined; closed > 0 {
					rate = float64(a.completed) / float64(closed)
				}
				report.Templates = append(report.Templates, templateStat{
					TemplateID: id, TemplateName: a.name, Documents: a.docs,
					Completed: a.completed, Declined: a.declined, Open: a.op,
					CompletionRate: rate,
				})
			}
			sort.Slice(report.Templates, func(i, j int) bool {
				if report.Templates[i].Documents != report.Templates[j].Documents {
					return report.Templates[i].Documents > report.Templates[j].Documents
				}
				return report.Templates[i].TemplateName < report.Templates[j].TemplateName
			})
			if limit > 0 && len(report.Templates) > limit {
				report.Templates = report.Templates[:limit]
			}
			if len(docs) == 0 {
				report.Note = "no documents in local store — run `pandadoc-cli sync` first"
			}

			w := cmd.OutOrStdout()
			if flags.asJSON || flags.agent || !isTerminal(w) {
				return printJSONFiltered(w, report, flags)
			}
			if len(report.Templates) == 0 {
				fmt.Fprintln(w, "No template data. Run `pandadoc-cli sync` first.")
				return nil
			}
			fmt.Fprintf(w, "Template velocity:\n\n")
			for _, t := range report.Templates {
				fmt.Fprintf(w, "  %-30s docs=%-4d completed=%-4d open=%-4d rate=%.0f%%\n",
					truncateField(t.TemplateName, 30), t.Documents, t.Completed, t.Open, t.CompletionRate*100)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (defaults to standard location)")
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum templates to return (0 = no limit)")
	return cmd
}
