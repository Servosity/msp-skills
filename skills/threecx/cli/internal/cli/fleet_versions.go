// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: fleet versions shows each PBX's build against the fleet's newest.
// pp:data-source live

package cli

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"threecx-pp-cli/internal/fleet"
)

func newNovelFleetVersionsCmd(flags *rootFlags) *cobra.Command {
	var scope fleetScope
	var behind bool

	cmd := &cobra.Command{
		Use:   "versions",
		Short: "Compare each phone system's 3CX build with the newest in your fleet, with pending and blocked updates (--behind for laggards only)",
		Long: `Each phone system's 3CX build against the newest build seen across your fleet,
with auto-update state, pending updates, and expired maintenance (which blocks
3CX updates).

Use this command to find PBXes behind the newest 3CX build in the fleet, with auto-update off, or with updates pending. Do NOT use this command for licence or maintenance renewal dates; use 'fleet licences' instead.

--behind keeps only PBXes that are behind, have pending updates, cannot update,
could not be read, or did not report what a verdict needs (shown as unknown).`,
		Example: strings.Trim(`
  threecx-cli fleet versions --behind --agent
  threecx-cli fleet versions --tag onprem --json`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "fleet versions")
			}
			pbxes, _, err := scope.resolve()
			if err != nil {
				return err
			}
			if len(pbxes) == 0 && emptyFleet(cmd, flags) {
				return nil
			}
			rows, failed := collect(cmd, &scope, flags, pbxes, []fleet.Request{{Path: "/SystemStatus"}, {Path: "/GetUpdates()"}},
				func(p fleet.PBX, bodies []json.RawMessage) ([]fleet.VersionRow, error) {
					var s fleet.SystemStatus
					var u fleet.UpdateList
					if err := decodeInto(bodies, &s, &u); err != nil {
						return nil, fmt.Errorf("decoding update status: %w", err)
					}
					return []fleet.VersionRow{fleet.VersionRowFor(p, s, u, time.Now().UTC())}, nil
				}, func(p fleet.PBX, err error) fleet.VersionRow {
					return fleet.VersionRow{PBX: p.Name, Error: err.Error()}
				})
			fleet.MarkDrift(rows)
			if behind {
				kept := make([]fleet.VersionRow, 0, len(rows))
				for _, r := range rows {
					if r.NeedsAttention() {
						kept = append(kept, r)
					}
				}
				rows = kept
			}
			err = render(cmd, flags, rows, []string{"pbx", "version", "newest_in_fleet", "behind",
				"pending_updates", "maintenance_expired", "auto_update", "error"},
				"Every PBX is on the newest build with nothing pending.", func(r fleet.VersionRow) map[string]any {
					auto := "-"
					if r.AutoUpdate != nil {
						auto = fmt.Sprint(*r.AutoUpdate)
					}
					var behindCell, pendingCell, maintCell any = triState(r.Behind), r.PendingUpdates, triState(r.MaintenanceExpired)
					if r.UnknownUpdates > 0 {
						pendingCell = fmt.Sprintf("%d (+%d unknown)", r.PendingUpdates, r.UnknownUpdates)
					}
					if r.Error != "" {
						// An unreadable PBX is not known to be current.
						behindCell, pendingCell, maintCell = "-", "-", "-"
					}
					return map[string]any{"pbx": r.PBX, "version": r.Version, "newest": r.NewestInFleet,
						"behind": behindCell, "pending": pendingCell, "auto_update": auto,
						"maint_expired": maintCell, "error": r.Error}
				})
			if err != nil {
				return err
			}
			return allFailed(failed, len(pbxes))
		},
	}
	addFleetScopeFlags(cmd, &scope)
	cmd.Flags().BoolVar(&behind, "behind", false, "Only PBXes that are behind, have pending updates, or cannot update")
	return cmd
}
