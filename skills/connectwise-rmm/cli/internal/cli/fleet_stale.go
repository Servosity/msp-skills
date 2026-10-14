// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live
// Reads the synced fleet list locally; availability, patches and sessions come from the live API.

package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"connectwise-rmm-pp-cli/internal/fleet"
)

type staleView struct {
	OlderThan        string           `json:"older_than"`
	ScannedEndpoints int              `json:"scanned_endpoints"`
	Note             string           `json:"note,omitempty"`
	Stale            []fleet.StaleRow `json:"stale"`
	FetchFailures    []fetchFailure   `json:"fetch_failures,omitempty"`
}

func newNovelFleetStaleCmd(flags *rootFlags) *cobra.Command {
	var olderThan, company, endpointType, dbPath string
	cmd := &cobra.Command{
		Use:   "stale",
		Short: "List endpoints offline or silent longer than --older-than across all companies, grouped by company",
		Long: strings.Trim(`
Every endpoint across all companies that is offline, or whose agent has been
silent longer than --older-than, grouped by company and ranked by silence.
Endpoints the heartbeat API did not answer for are listed with no_heartbeat.

Reads endpoints, companies and sites from the local store (run sync first) and
checks live availability through the heartbeat API, 400 endpoints per call.

Use this command for fleet-wide offline or silent agents. Do NOT use this
command for one named endpoint's status; use 'fleet brief' instead.`, "\n"),
		Example: strings.Trim(`
  connectwise-rmm-cli fleet stale --older-than 24h --agent
  connectwise-rmm-cli fleet stale --older-than 7d --company "Acme Corp" --type Server
  connectwise-rmm-cli fleet stale --agent --select stale.name,stale.company,stale.silent_hours`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "fleet stale")
			}
			threshold, err := parseDurationFlag("older-than", olderThan)
			if err != nil {
				return err
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			view := staleView{OlderThan: olderThan, Stale: make([]fleet.StaleRow, 0)}
			db, missing, err := openFleetStore(ctx, cmd, dbPath)
			if err != nil {
				return err
			}
			if missing {
				view.Note = noMirrorNote
				return emptyResult(cmd, flags, view)
			}
			defer db.Close()
			if !hintIfUnsynced(cmd, db, "endpoints") {
				hintIfStale(cmd, db, "endpoints", flags.maxAge)
			}
			eps, err := loadFleet(db)
			if err != nil {
				return err
			}
			eps = filterFleet(eps, company, endpointType)
			view.ScannedEndpoints = len(eps)
			view.Note = unsyncedNote(len(eps))
			if len(eps) == 0 {
				return printStale(cmd, flags, view)
			}
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			hb, failures := fetchHeartbeats(ctx, c, endpointIDs(eps))
			if err := allHeartbeatsFailed(hb, failures); err != nil {
				return err
			}
			warnFailures(cmd, "heartbeat", failures, len(eps))
			view.FetchFailures = failures
			view.Stale = fleet.Stale(eps, hb, time.Now().UTC(), threshold)
			// Endpoints in a failed batch or a failedRecord have unknown
			// availability; drop them rather than report them stale.
			view.Stale = dropUnknown(view.Stale, failures)
			return printStale(cmd, flags, view)
		},
	}
	cmd.Flags().StringVar(&olderThan, "older-than", "24h", "Silence threshold, e.g. 24h, 7d, 2w")
	cmd.Flags().StringVar(&company, "company", "", "Limit to one company (ID or name)")
	cmd.Flags().StringVar(&endpointType, "type", "", "Limit to one endpoint type, e.g. Server or Desktop")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: the CLI's local store)")
	return cmd
}

func dropUnknown(rows []fleet.StaleRow, failures []fetchFailure) []fleet.StaleRow {
	unknown := map[string]bool{}
	for _, f := range failures {
		for _, id := range f.IDs {
			unknown[id] = true
		}
	}
	out := make([]fleet.StaleRow, 0, len(rows))
	for _, r := range rows {
		if !unknown[r.ID] {
			out = append(out, r)
		}
	}
	return out
}

func printStale(cmd *cobra.Command, flags *rootFlags, v staleView) error {
	if !wantsHumanTable(cmd.OutOrStdout(), flags) {
		return printJSONFiltered(cmd.OutOrStdout(), v, flags)
	}
	if len(v.Stale) == 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "No offline or silent agents among %d endpoints (threshold %s).\n", v.ScannedEndpoints, v.OlderThan)
		return nil
	}
	rows := make([]map[string]any, 0, len(v.Stale))
	for _, r := range v.Stale {
		rows = append(rows, map[string]any{"company": r.CompanyName, "site": r.SiteName, "endpoint": firstNonEmpty(r.Name, r.ID), "type": r.Type, "online": r.Available, "last_seen": r.LastSeen, "silent_h": r.SilentHours})
	}
	return printAutoTable(cmd.OutOrStdout(), rows)
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
