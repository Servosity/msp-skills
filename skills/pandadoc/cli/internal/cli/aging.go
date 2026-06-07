// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"sort"
	"time"

	"github.com/spf13/cobra"
)

type agingBucket struct {
	Bucket string `json:"bucket"`
	Count  int    `json:"count"`
}

type agingStatusRow struct {
	Status     string        `json:"status"`
	Count      int           `json:"count"`
	MedianDays int           `json:"median_days"`
	OldestDays int           `json:"oldest_days"`
	Buckets    []agingBucket `json:"buckets"`
}

type agingReport struct {
	Total int              `json:"total"`
	Rows  []agingStatusRow `json:"rows"`
	Note  string           `json:"note,omitempty"`
}

// agingBucketLabels is the ordered display list. agingBucketLabel MUST return
// only values from this slice — the render loop iterates it, so a label that
// is produced but not listed here silently disappears from output.
var agingBucketLabels = []string{"0-7d", "8-30d", "31-90d", "91d+"}

func agingBucketLabel(days int) string {
	switch {
	case days <= 7:
		return agingBucketLabels[0]
	case days <= 30:
		return agingBucketLabels[1]
	case days <= 90:
		return agingBucketLabels[2]
	default:
		return agingBucketLabels[3]
	}
}

// newNovelAgingCmd implements the "aging" transcendence command: time-in-status
// distribution across all synced documents.
// pp:data-source local
func newNovelAgingCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	cmd := &cobra.Command{
		Use:         "aging",
		Short:       "Show how long each document has sat in its current status, bucketed by age.",
		Long:        "Bucket every document by how long it has sat in its current status — where the pipeline piles up.\n\nUse this command to bucket ALL documents by time-in-current-status. Do NOT use it to find only sent-but-unsigned deals past a threshold; use 'stalled' instead.\n\nReads the local store — run `sync` first. Needs date_modified across all docs joined to status, which no single API call returns.",
		Example:     "  pandadoc-cli aging --agent",
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

			now := time.Now().UTC()
			byStatus := map[string][]int{}
			for _, d := range docs {
				activity := d.lastActivity()
				if activity.IsZero() {
					continue
				}
				age := int(now.Sub(activity).Hours() / 24)
				if age < 0 {
					age = 0
				}
				byStatus[shortStatus(d.Status)] = append(byStatus[shortStatus(d.Status)], age)
			}

			report := agingReport{Total: len(docs), Rows: make([]agingStatusRow, 0, len(byStatus))}
			for status, ages := range byStatus {
				sort.Ints(ages)
				bucketCounts := map[string]int{}
				for _, a := range ages {
					bucketCounts[agingBucketLabel(a)]++
				}
				buckets := make([]agingBucket, 0)
				for _, lbl := range agingBucketLabels {
					if c, ok := bucketCounts[lbl]; ok {
						buckets = append(buckets, agingBucket{Bucket: lbl, Count: c})
					}
				}
				report.Rows = append(report.Rows, agingStatusRow{
					Status:     status,
					Count:      len(ages),
					MedianDays: ages[len(ages)/2],
					OldestDays: ages[len(ages)-1],
					Buckets:    buckets,
				})
			}
			sort.Slice(report.Rows, func(i, j int) bool { return report.Rows[i].Count > report.Rows[j].Count })
			if len(docs) == 0 {
				report.Note = "no documents in local store — run `pandadoc-cli sync` first"
			}

			w := cmd.OutOrStdout()
			if flags.asJSON || flags.agent || !isTerminal(w) {
				return printJSONFiltered(w, report, flags)
			}
			fmt.Fprintf(w, "Document aging (%d total)\n\n", report.Total)
			for _, r := range report.Rows {
				fmt.Fprintf(w, "  %-14s n=%-4d median=%-4dd oldest=%-4dd\n", r.Status, r.Count, r.MedianDays, r.OldestDays)
			}
			if report.Note != "" {
				fmt.Fprintf(w, "\n  %s\n", report.Note)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (defaults to standard location)")
	return cmd
}
