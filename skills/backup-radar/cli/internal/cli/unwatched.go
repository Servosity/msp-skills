// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source local

package cli

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"backup-radar-pp-cli/internal/radar"
)

type unwatchedSkip struct {
	Reason string `json:"reason"`
	Why    string `json:"why"`
}

type unwatchedView struct {
	Rows    []radar.UnwatchedRow `json:"rows"`
	Counts  map[string]int       `json:"counts"`
	Skipped []unwatchedSkip      `json:"skipped"`
	Sources radarSources         `json:"sources"`
}

func newNovelUnwatchedCmd(flags *rootFlags) *cobra.Command {
	var reasonFlag, client, dbPath string
	var limit int

	cmd := &cobra.Command{
		Use:   "unwatched",
		Short: "List backups nobody acts on (inactive, unmapped, ticketless, no PSA company, warnings counted as success) from the local store",
		Long: `List every blind spot with a reason code:
  inactive    results arrive but the backup was never activated (v2)
  unmapped    device or company not mapped to ScalePad (v3)
  ticketless  failing or no-result with zero open tickets (v2)
  no-psa      no PSA ticketing company, so failures never ticket (v2)
  masked      warnings counted as success by policy (v2)
Reasons whose collection was never synced are listed under "skipped"
instead of reported as zero. One backup can appear once per reason.

Use this command to find backups nobody is watching inside Backup Radar.
Do NOT use this command for a two-way reconciliation of a vendor's device list against Backup Radar; use 'coverage diff' instead.
Do NOT use it for silent backups that are retire candidates; use 'stale' instead.
Do NOT use it to rank today's failures by severity; use 'check' instead.`,
		Example: `  backup-radar-cli unwatched
  backup-radar-cli unwatched --reason inactive,ticketless --agent
  backup-radar-cli unwatched --reason unmapped --client Contoso --json`,
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local", "mcp:write-flags": "csv,plain,quiet"}, // HAND-FIX(radar-machine-modes): refused modes leave the MCP schema
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := refuseTabularModes(flags, "unwatched"); err != nil {
				return err
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "unwatched")
			}
			reasons := radar.AllReasons
			if reasonFlag != "" {
				reasons = splitCSVFlag(reasonFlag)
				for _, r := range reasons {
					if !slices.Contains(radar.AllReasons, r) {
						return usageErr(fmt.Errorf("unknown --reason %q; use any of %s", r, strings.Join(radar.AllReasons, ",")))
					}
				}
			}
			snap, ok, err := loadRadarSnapshot(cmd, flags, dbPath)
			if err != nil {
				return err
			}
			colls := make([]string, 0, len(reasons))
			for _, r := range reasons {
				colls = append(colls, radar.ReasonCollection(r))
			}
			view := unwatchedView{Rows: make([]radar.UnwatchedRow, 0), Counts: map[string]int{}, Skipped: make([]unwatchedSkip, 0), Sources: sourcesOf(snap, colls...)}
			active := make([]string, 0, len(reasons))
			for _, r := range reasons {
				coll := radar.ReasonCollection(r)
				if snap.Synced[coll] {
					active = append(active, r)
					view.Counts[r] = 0 // a checked reason with no rows still reports 0
					continue
				}
				key := "BACKUP_RADAR_V2_API_KEY"
				if coll == "devices" {
					key = "BACKUP_RADAR_API_KEY"
				}
				view.Skipped = append(view.Skipped, unwatchedSkip{Reason: r, Why: fmt.Sprintf("%s was never synced; set %s and run 'backup-radar-cli sync --resources %s'", coll, key, coll)})
			}
			if ok {
				for _, row := range radar.Unwatched(snap, active, time.Now().UTC()) {
					if !radar.ClientMatches(client, row.Client, row.BRCompany) {
						continue
					}
					view.Rows = append(view.Rows, row)
					view.Counts[row.Reason]++
				}
			}
			view.Counts["total"] = len(view.Rows)
			if limit > 0 && len(view.Rows) > limit {
				view.Rows = view.Rows[:limit]
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFilteredKeep(cmd.OutOrStdout(), view, flags, radarKeep(radar.UnwatchedRow{})...)
			}
			for _, s := range view.Skipped {
				fmt.Fprintf(cmd.ErrOrStderr(), "skipped %s: %s\n", scrubRadarText(s.Reason), scrubRadarText(s.Why))
			}
			if len(view.Rows) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No unwatched backups found in the local store.")
				printSourcesNote(cmd.OutOrStdout(), view.Sources.Note)
				return nil
			}
			table := make([][]string, 0, len(view.Rows))
			for _, r := range view.Rows {
				table = append(table, []string{r.Reason, r.Client, r.Device, r.Job, r.Detail})
			}
			printSourcesNote(cmd.ErrOrStderr(), view.Sources.Note)
			return printRadarTable(cmd.OutOrStdout(), []string{"reason", "client", "device", "job", "detail"}, table)
		},
	}
	cmd.Flags().StringVar(&reasonFlag, "reason", "", "Comma-separated reasons to report: inactive,unmapped,ticketless,no-psa,masked (default: all)")
	cmd.Flags().StringVar(&client, "client", "", "Only clients whose name contains this text (case-insensitive)")
	cmd.Flags().IntVar(&limit, "limit", 500, "Maximum rows to return (0 = all)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: the synced local store)")
	return cmd
}
