// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live
// Reads the synced fleet list locally; availability, patches and sessions come from the live API.

package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"connectwise-rmm-pp-cli/internal/fleet"
)

type rebootView struct {
	ScannedEndpoints int               `json:"scanned_endpoints"`
	CapHit           bool              `json:"scan_cap_hit,omitempty"`
	Counts           map[string]int    `json:"counts"`
	Note             string            `json:"note,omitempty"`
	Endpoints        []fleet.RebootRow `json:"endpoints"`
	FetchFailures    []fetchFailure    `json:"fetch_failures,omitempty"`
}

func newNovelFleetRebootDueCmd(flags *rootFlags) *cobra.Command {
	var company, cacheTTL, dbPath string
	var maxEndpoints int
	var refresh bool
	cmd := &cobra.Command{
		Use:   "reboot-due",
		Short: "List endpoints with pending reboots, split into reboot now, coordinate, offline, and availability unknown",
		Long: strings.Trim(`
Endpoints whose patch summary shows pending reboots, split into four buckets:
  reboot_now            online, nobody actively logged on
  coordinate            online with an active user session (or sessions unknown)
  offline               agent offline or missing from the heartbeat
  availability_unknown  the heartbeat call failed for this endpoint

Patch summaries come from the local cache (refreshed per --cache-ttl);
availability and sessions are read live for the pending endpoints only.

Use this command to get machines needing a reboot and whether they can be
rebooted now. Do NOT use this command for compliance ranking; use 'fleet
patch-posture' instead.`, "\n"),
		Example: strings.Trim(`
  connectwise-rmm-cli fleet reboot-due --agent
  connectwise-rmm-cli fleet reboot-due --company "Acme Corp" --select endpoints.name,endpoints.bucket`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "fleet reboot-due")
			}
			ttl, err := parseDurationFlag("cache-ttl", cacheTTL)
			if err != nil {
				return err
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			view := rebootView{Counts: map[string]int{fleet.RebootNow: 0, fleet.Coordinate: 0, fleet.Offline: 0, fleet.Unknown: 0}, Endpoints: make([]fleet.RebootRow, 0)}
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
			ps, err := loadPatchScope(ctx, cmd, flags, db, company, maxEndpoints, ttl, refresh)
			if err != nil {
				return err
			}
			view.ScannedEndpoints, view.CapHit, view.FetchFailures = len(ps.eps), ps.capHit, ps.failures
			view.Note = unsyncedNote(len(ps.eps))
			pending := make([]fleet.Endpoint, 0)
			for _, e := range ps.eps {
				if ps.sums[e.ID].PendingReboot > 0 {
					pending = append(pending, e)
				}
			}
			if len(pending) > 0 {
				c, err := flags.newClient()
				if err != nil {
					return err
				}
				hb, hbFail := fetchHeartbeats(ctx, c, endpointIDs(pending))
				if err := allHeartbeatsFailed(hb, hbFail); err != nil {
					return err
				}
				online := make([]string, 0)
				for _, e := range pending {
					if hb[e.ID].Available {
						online = append(online, e.ID)
					}
				}
				// Sessions change minute to minute: always read them live.
				raw, stFail := cachedEndpointData(ctx, cmd, db, c, cacheSystemState, online, 0, true, systemStatePath)
				states, badStates := systemStates(raw)
				stFail = append(stFail, badStates...)
				warnFailures(cmd, "system state", stFail, len(online))
				warnFailures(cmd, "heartbeat", hbFail, len(pending))
				view.FetchFailures = append(append(view.FetchFailures, hbFail...), stFail...)
				view.Endpoints = fleet.RebootTriage(pending, ps.sums, hb, unknownIDs(hbFail), states)
				for _, r := range view.Endpoints {
					view.Counts[r.Bucket]++
				}
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), view, flags)
			}
			if len(view.Endpoints) == 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "No pending reboots among %d endpoints.\n", view.ScannedEndpoints)
				return nil
			}
			rows := make([]map[string]any, 0, len(view.Endpoints))
			for _, r := range view.Endpoints {
				users := make([]string, 0, len(r.LoggedOn))
				for _, u := range r.LoggedOn {
					users = append(users, u.Username)
				}
				rows = append(rows, map[string]any{"bucket": r.Bucket, "company": r.CompanyName, "endpoint": firstNonEmpty(r.Name, r.ID), "pending": r.PendingReboot, "users": strings.Join(users, ", ")})
			}
			return printAutoTable(cmd.OutOrStdout(), rows)
		},
	}
	cmd.Flags().StringVar(&company, "company", "", "Limit to one company (ID or name)")
	cmd.Flags().StringVar(&cacheTTL, "cache-ttl", "12h", "Reuse cached patch summaries younger than this")
	cmd.Flags().BoolVar(&refresh, "refresh", false, "Ignore cached patch summaries and re-read every endpoint")
	cmd.Flags().IntVar(&maxEndpoints, "max-endpoints", 500, "Maximum endpoints to read patch summaries for (0 = no cap)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: the CLI's local store)")
	return cmd
}
