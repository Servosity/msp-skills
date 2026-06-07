// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"sync"

	"github.com/spf13/cobra"

	"pandadoc-pp-cli/internal/cliutil"
)

type reminderGap struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Status      string `json:"status"`
	DaysIdle    int    `json:"days_idle"`
	ReminderOff bool   `json:"reminder_off"`
}

type reminderFetchFailure struct {
	ID    string `json:"id"`
	Error string `json:"error"`
}

type reminderGapsReport struct {
	Gaps             []reminderGap          `json:"gaps"`
	ScannedDocuments int                    `json:"scanned_documents"`
	MaxScanDocs      int                    `json:"max_scan_docs"`
	FetchFailures    []reminderFetchFailure `json:"fetch_failures,omitempty"`
	Note             string                 `json:"note,omitempty"`
}

// newNovelReminderGapsCmd implements the "reminder-gaps" transcendence
// command: finds sent-but-incomplete documents whose auto-reminders are
// disabled, so PandaDoc isn't nudging the signer for you. Selects open
// documents from the local store, then fans out live auto-reminder lookups —
// no API endpoint cross-references the two.
// pp:data-source live
func newNovelReminderGapsCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	var maxScanDocs int
	var limit int
	cmd := &cobra.Command{
		Use:         "reminder-gaps",
		Short:       "Find sent-but-incomplete documents that have no active auto-reminder set.",
		Long:        "Find open (sent/viewed) documents whose auto-reminders are disabled — the deals where PandaDoc is not nudging the signer for you.\n\nUse this command to find open documents missing an automated REMINDER. Do NOT use it to find documents past a staleness threshold regardless of reminders; use 'stalled' instead.\n\nSelects open documents from the local store (run `sync` first), then calls the live auto-reminders endpoint per document — it requires API credentials and respects --max-scan-docs to bound the fan-out.",
		Example:     "  pandadoc-cli reminder-gaps --max-scan-docs 25 --agent",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would scan open documents and fetch live auto-reminder settings for up to", maxScanDocs, "documents")
				return nil
			}
			if err := validateDataSourceStrategy(flags, "live"); err != nil {
				return err
			}
			if maxScanDocs <= 0 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--max-scan-docs must be positive"))
			}
			// Live dogfood runs a flat per-command timeout; curtail the fan-out.
			if cliutil.IsDogfoodEnv() && maxScanDocs > 3 {
				maxScanDocs = 3
			}

			db, _, err := openNovelStore(cmd, flags, dbPath, "documents")
			if err != nil {
				return err
			}
			docs, err := loadDocumentsFromStore(db)
			// Close the store eagerly before the live API fan-out so the DB
			// handle isn't held across the network loop; the close error is
			// not actionable here (reads are already done).
			_ = db.Close()
			if err != nil {
				return err
			}

			// Open, already-sent documents are the only ones whose reminders
			// matter; drafts have nobody to remind and terminal docs are done.
			open := docs[:0:0]
			for _, d := range docs {
				if isTerminalStatus(d.Status) {
					continue
				}
				st := normalizeStatus(d.Status)
				if st == "document.draft" || st == "document.uploaded" {
					continue
				}
				open = append(open, d)
			}
			// Coldest first so the bounded scan inspects the riskiest deals.
			sort.Slice(open, func(i, j int) bool {
				return ageDays(open[i].lastActivity()) > ageDays(open[j].lastActivity())
			})
			scanCapHit := len(open) > maxScanDocs
			if scanCapHit {
				open = open[:maxScanDocs]
			}

			report := reminderGapsReport{
				Gaps:             make([]reminderGap, 0),
				ScannedDocuments: len(open),
				MaxScanDocs:      maxScanDocs,
				FetchFailures:    make([]reminderFetchFailure, 0),
			}
			if len(open) == 0 {
				if len(docs) == 0 {
					report.Note = "no documents in local store — run `pandadoc-cli sync` first"
				} else {
					report.Note = "no open sent/viewed documents — nothing needs a reminder"
				}
				return printJSONFiltered(cmd.OutOrStdout(), report, flags)
			}

			c, err := flags.newClient()
			if err != nil {
				return err
			}

			type fetchResult struct {
				idx     int
				enabled bool
				err     error
			}
			results := make(chan fetchResult, len(open))
			var wg sync.WaitGroup
			for idx, d := range open {
				wg.Add(1)
				go func() {
					defer wg.Done()
					data, err := c.Get(cmd.Context(), "/documents/"+url.PathEscape(d.ID)+"/auto-reminders", nil)
					if err != nil {
						results <- fetchResult{idx: idx, err: err}
						return
					}
					var settings struct {
						Enabled bool `json:"enabled"`
					}
					if err := json.Unmarshal(data, &settings); err != nil {
						results <- fetchResult{idx: idx, err: fmt.Errorf("parsing auto-reminder settings: %w", err)}
						return
					}
					results <- fetchResult{idx: idx, enabled: settings.Enabled}
				}()
			}
			go func() {
				wg.Wait()
				close(results)
			}()

			enabledByIdx := make([]bool, len(open))
			errByIdx := make([]error, len(open))
			for r := range results {
				enabledByIdx[r.idx] = r.enabled
				errByIdx[r.idx] = r.err
			}
			for idx, d := range open {
				if errByIdx[idx] != nil {
					report.FetchFailures = append(report.FetchFailures, reminderFetchFailure{
						ID: d.ID, Error: errByIdx[idx].Error(),
					})
					continue
				}
				if !enabledByIdx[idx] {
					report.Gaps = append(report.Gaps, reminderGap{
						ID:          d.ID,
						Name:        d.Name,
						Status:      shortStatus(d.Status),
						DaysIdle:    ageDays(d.lastActivity()),
						ReminderOff: true,
					})
				}
			}
			if len(report.FetchFailures) > 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %d of %d reminder lookups failed; gaps computed over the remaining %d documents\n",
					len(report.FetchFailures), len(open), len(open)-len(report.FetchFailures))
			}
			sort.Slice(report.Gaps, func(i, j int) bool {
				if report.Gaps[i].DaysIdle != report.Gaps[j].DaysIdle {
					return report.Gaps[i].DaysIdle > report.Gaps[j].DaysIdle
				}
				return report.Gaps[i].ID < report.Gaps[j].ID
			})
			if limit > 0 && len(report.Gaps) > limit {
				report.Gaps = report.Gaps[:limit]
			}
			if len(report.Gaps) == 0 && len(report.FetchFailures) == 0 {
				if scanCapHit {
					report.Note = fmt.Sprintf("scanned the %d coldest open documents without finding a reminder gap; raise --max-scan-docs to widen the scan", len(open))
				} else {
					report.Note = "every open document has auto-reminders enabled"
				}
			}

			w := cmd.OutOrStdout()
			if flags.asJSON || flags.agent || !isTerminal(w) {
				return printJSONFiltered(w, report, flags)
			}
			if len(report.Gaps) == 0 {
				fmt.Fprintln(w, report.Note)
				return nil
			}
			fmt.Fprintf(w, "Reminder gaps (auto-reminders OFF), %d of %d scanned docs:\n\n", len(report.Gaps), report.ScannedDocuments)
			for _, g := range report.Gaps {
				fmt.Fprintf(w, "  %-40s %-9s idle=%dd\n", truncateField(g.Name, 40), g.Status, g.DaysIdle)
			}
			if len(report.FetchFailures) > 0 {
				fmt.Fprintf(w, "\n  partial results: %d lookups failed\n", len(report.FetchFailures))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (defaults to standard location)")
	cmd.Flags().IntVar(&maxScanDocs, "max-scan-docs", 25, "Maximum open documents to fetch live reminder settings for")
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum gaps to return (0 = no limit)")
	return cmd
}
