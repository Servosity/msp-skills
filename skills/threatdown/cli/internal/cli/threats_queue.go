// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source local

package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"threatdown-pp-cli/internal/cliutil"
	"threatdown-pp-cli/internal/fleet"
)

type threatsQueueView struct {
	Source string           `json:"source"`
	Note   string           `json:"note,omitempty"`
	Since  string           `json:"since"`
	Count  int              `json:"count"`
	Queue  []fleet.QueueRow `json:"queue"`
}

func newNovelThreatsQueueCmd(flags *rootFlags) *cobra.Command {
	var since, siteFilter, dbPath string
	var limit int

	cmd := &cobra.Command{
		Use:   "queue",
		Short: "Prioritized list of infected or suspicious endpoints with online and isolated state and a suggested response.",
		Long: `Use this command for a prioritized per-endpoint list of infected or suspicious endpoints with their online/isolated state and a suggested response. Do NOT use this command to find threats spreading across sites; use 'threats spread' instead.

Joins endpoints and detections stored by 'fleet sync'. Ranking: active
threats first (online and not isolated highest, since they can still
spread), then open suspicious activity, then forgotten isolations (isolated,
clean, no detections in the window). Each row carries the next command to run.`,
		Example: strings.Trim(`
  threatdown-cli threats queue --since 7d --agent
  threatdown-cli threats queue --site "Acme" --json --select queue.host_name,queue.kind,queue.suggested_command`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "threats queue")
			}
			window, err := cliutil.ParseDurationLoose(since)
			if err != nil || window <= 0 {
				return usageErr(fmt.Errorf("--since must be a duration like 7d, got %q", since))
			}
			view := threatsQueueView{Source: "local", Since: since, Queue: make([]fleet.QueueRow, 0)}
			data, ok, err := loadThreatData(cmd, tdDBPath(dbPath), true, time.Now().UTC().Add(-window))
			if err != nil {
				return err
			}
			if !ok || !data.synced {
				view.Note = "no detections synced yet; run " + tdFleetSyncCommand
			} else if data.coverage != "" {
				view.Note = data.coverage
			}
			view.Note = joinNotes(view.Note, data.incomplete)
			if ok {
				sites, err := scopeSites(data.sites, siteFilter)
				if err != nil {
					return err
				}
				if siteFilter != "" {
					view.Note = joinNotes(view.Note, accountlessNote(sites))
				}
				keep := map[string]bool{}
				for _, a := range fleet.AccountIDs(sites, "") {
					keep[a] = true
				}
				eps := data.endpoints[:0:0]
				for _, e := range data.endpoints {
					if siteFilter == "" || keep[e.AccountID] {
						eps = append(eps, e)
					}
				}
				view.Queue = fleet.Queue(eps, data.detections, fleet.SiteIndex(data.sites), time.Now().UTC().Add(-window), tdBinary)
				// "Forgotten" means no detections in the window; without synced
				// history covering the window that is unproven, so never offer
				// a restore for it.
				if !data.synced || data.coverage != "" || data.behind || data.incomplete != "" {
					kept, dropped := view.Queue[:0], 0
					for _, q := range view.Queue {
						if q.Kind == "forgotten-isolation" {
							dropped++
							continue
						}
						q.Rank = len(kept) + 1
						kept = append(kept, q)
					}
					view.Queue = kept
					if dropped > 0 {
						view.Note = strings.TrimPrefix(view.Note+fmt.Sprintf("; %d isolated endpoint(s) not listed as forgotten-isolation: detection history does not cover the window, predates the endpoint snapshot, or the latest fleet sync was incomplete", dropped), "; ")
					}
				}
			}
			if limit > 0 && len(view.Queue) > limit {
				view.Queue = view.Queue[:limit]
			}
			view.Count = len(view.Queue)
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFilteredKeep(cmd.OutOrStdout(), view, flags, "last_threat", "suggested_command", "note")
			}
			if view.Note != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "note: %s\n", view.Note)
			}
			if view.Count == 0 {
				if view.Note == "" {
					fmt.Fprintln(cmd.OutOrStdout(), "Nothing to triage: no infected, suspicious or forgotten-isolated endpoints.")
				}
				return nil
			}
			return printColumns(cmd.OutOrStdout(), view.Queue, "rank", "kind", "site", "host_name", "online", "isolated", "infection_count", "detections_in_window", "last_threat", "suggested_command")
		},
	}
	cmd.Flags().StringVar(&since, "since", "7d", "Window of detections that count toward ranking (e.g. 7d)")
	cmd.Flags().StringVar(&siteFilter, "site", "", "Only sites whose name contains this text")
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum rows to return (0 = all)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: the store 'fleet sync' writes)")
	return cmd
}
