// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: fleet patch reports patch-management status per client instance.
// pp:data-source live

package cli

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"jamf-pro-pp-cli/internal/fleet"
)

func newNovelFleetPatchCmd(flags *rootFlags) *cobra.Command {
	var behindOnly bool
	var scope fleetScope

	cmd := &cobra.Command{
		Use:   "patch",
		Short: "Patch titles behind latest, failing patch policies and failed managed OS updates for every client in one table",
		Long: `Patch titles behind latest, failing patch policies and failed managed OS updates for every client in one table.

Use this command for titles configured in Jamf patch management, failing patch policies, and managed OS update errors. Do NOT use this command for an app or version that is not a configured patch title; use 'fleet app' instead.

Per instance it reads /api/v3/patch-software-title-configurations and each
title's patch-summary (latestVersion, upToDate, outOfDate as Jamf reports them),
the failed counts of /api/v2/patch-policies, and the managed software update
statuses in ERROR, DOWNLOAD_FAILED or INSTALL_FAILED. Rows of kind patch_title
are one per title (devices up to date / out of date, failed patch-policy
installs); one row per instance of kind os_updates carries failed_os_updates.`,
		Example: strings.Trim(`
  jamf-pro-cli fleet patch --behind-only --agent
  jamf-pro-cli fleet patch --only acme --json`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "fleet patch")
			}
			instances, err := scope.resolve()
			if err != nil {
				return err
			}
			if len(instances) == 0 && emptyFleet(cmd, flags) {
				return nil
			}
			rows, failed := collectFn(cmd, &scope, flags, instances,
				func(fetch fetchFunc, p fleet.Instance) ([]fleet.PatchRow, error) {
					bodies, err := fetch(
						jamfGet("/api/v3/patch-software-title-configurations"),
						jamfGet("/api/v2/patch-policies", "page-size", "100"),
						jamfGet("/api/v1/managed-software-updates/update-statuses", "filter", fleet.FailedUpdateFilter, "page-size", "100"),
					)
					if err != nil {
						return nil, err
					}
					var titles []fleet.PatchTitle
					if err := json.Unmarshal(bodies[0], &titles); err != nil || titles == nil {
						return nil, fmt.Errorf("patch-software-title-configurations: unexpected shape: not a list")
					}
					for i, t := range titles {
						if t.ID == "" {
							return nil, fmt.Errorf("patch-software-title-configurations: entry %d has no id", i)
						}
					}
					policies, err := fleet.Results[fleet.PatchPolicy](bodies[1], "id", "softwareTitleConfigurationId", "failed")
					if err != nil {
						return nil, fmt.Errorf("patch-policies: %w", err)
					}
					for i, pp := range policies {
						if strings.TrimSpace(pp.SoftwareTitleConfigurationID) == "" {
							return nil, fmt.Errorf("patch-policies: result %d has no softwareTitleConfigurationId, so its failures cannot be attributed", i)
						}
					}
					updates, err := fleet.Results[fleet.UpdateStatus](bodies[2], "status")
					if err != nil {
						return nil, fmt.Errorf("update-statuses: %w", err)
					}
					summaries := make([]fleet.PatchSummary, len(titles))
					if len(titles) > 0 {
						reqs := make([]fleet.Request, len(titles))
						for i, t := range titles {
							reqs[i] = jamfGet("/api/v3/patch-software-title-configurations/" + t.ID + "/patch-summary")
						}
						sb, err := fetch(reqs...)
						if err != nil {
							return nil, fmt.Errorf("patch-summary: %w", err)
						}
						for i := range sb {
							if err := fleet.DecodeObject(sb[i], &summaries[i], "upToDate", "outOfDate"); err != nil {
								return nil, fmt.Errorf("patch-summary for title %d: unexpected shape: %w", i, err)
							}
						}
					}
					return fleet.PatchRows(p.Name, titles, summaries, policies, updates), nil
				},
				func(p fleet.Instance, err error) fleet.PatchRow {
					return fleet.PatchRow{Instance: p.Name, Error: err.Error()}
				})
			if behindOnly {
				kept := rows[:0]
				for _, r := range rows {
					if r.Behind() {
						kept = append(kept, r)
					}
				}
				rows = kept
			}
			cols := []string{"instance", "kind", "title", "latest", "up_to_date", "out_of_date", "on_latest", "policy_failures", "failed_os_updates", "error"}
			if err := render(cmd, flags, rows, []string{"instance", "kind", "error"}, "Every configured patch title is current.", cols, func(r fleet.PatchRow) map[string]any {
				pct := ""
				if r.PercentLatest != nil {
					pct = strconv.Itoa(*r.PercentLatest) + "%"
				}
				return map[string]any{"instance": r.Instance, "kind": r.Kind, "title": r.Title, "latest": r.LatestVersion, "up_to_date": r.UpToDate,
					"out_of_date": r.OutOfDate, "on_latest": pct, "policy_failures": r.PolicyFailures, "failed_os_updates": r.FailedUpdates, "error": r.Error}
			}); err != nil {
				return err
			}
			return allFailed(failed, len(instances))
		},
	}
	cmd.Flags().BoolVar(&behindOnly, "behind-only", false, "Only rows that need attention (out-of-date devices, failed patch policies or OS updates)")
	addFleetScopeFlags(cmd, &scope)
	return cmd
}
