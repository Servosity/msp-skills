// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"
)

type pipelineStage struct {
	Status string `json:"status"`
	Count  int    `json:"count"`
}

type pipelineReport struct {
	Total     int             `json:"total"`
	Stages    []pipelineStage `json:"stages"`
	Open      int             `json:"open"`
	Completed int             `json:"completed"`
	Declined  int             `json:"declined"`
	SignRate  float64         `json:"sign_rate"`
	Note      string          `json:"note,omitempty"`
}

// newNovelPipelineCmd implements the "pipeline" transcendence command:
// a status-distribution funnel across all synced documents.
// pp:data-source local
func newNovelPipelineCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	cmd := &cobra.Command{
		Use:         "pipeline",
		Short:       "See your whole document funnel at a glance — how many are in draft, sent, viewed, completed, or declined.",
		Long:        "Show the whole document funnel at a glance: how many documents sit in each status across every synced document, plus a view-to-sign rate.\n\nReads the local store — run `sync` first. The PandaDoc API returns one page of documents at a time with no status rollup; this aggregates the full corpus locally.",
		Example:     "  pandadoc-cli pipeline --agent",
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

			counts := map[string]int{}
			for _, d := range docs {
				counts[d.Status]++
			}
			stages := make([]pipelineStage, 0, len(counts))
			for s, c := range counts {
				stages = append(stages, pipelineStage{Status: shortStatus(s), Count: c})
			}
			sort.Slice(stages, func(i, j int) bool {
				if stages[i].Count != stages[j].Count {
					return stages[i].Count > stages[j].Count
				}
				return stages[i].Status < stages[j].Status
			})

			report := pipelineReport{Total: len(docs), Stages: stages}
			for _, d := range docs {
				switch normalizeStatus(d.Status) {
				case "document.completed", "document.paid":
					report.Completed++
				case "document.declined", "document.voided", "document.rejected", "document.expired":
					report.Declined++
				default:
					report.Open++
				}
			}
			if reached := report.Completed + report.Declined; reached > 0 {
				report.SignRate = float64(report.Completed) / float64(reached)
			}
			if len(docs) == 0 {
				report.Note = "no documents in local store — run `pandadoc-cli sync` first"
			}

			w := cmd.OutOrStdout()
			if flags.asJSON || flags.agent || !isTerminal(w) {
				return printJSONFiltered(w, report, flags)
			}
			fmt.Fprintf(w, "Document pipeline — %d total\n\n", report.Total)
			for _, s := range report.Stages {
				fmt.Fprintf(w, "  %-22s %d\n", s.Status, s.Count)
			}
			fmt.Fprintf(w, "\n  open=%d completed=%d declined=%d sign_rate=%.0f%%\n",
				report.Open, report.Completed, report.Declined, report.SignRate*100)
			if report.Note != "" {
				fmt.Fprintf(w, "\n  %s\n", report.Note)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (defaults to standard location)")
	return cmd
}
