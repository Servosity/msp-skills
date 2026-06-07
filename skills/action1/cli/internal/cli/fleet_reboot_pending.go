// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"sort"

	"github.com/spf13/cobra"
)

type rebootPendingRow struct {
	OrganizationID string `json:"organization_id"`
	EndpointID     string `json:"endpoint_id"`
	Name           string `json:"name"`
	OS             string `json:"os"`
	MissingCrit    int    `json:"missing_critical_updates"`
	LastSeen       string `json:"last_seen"`
	OnlineStatus   string `json:"online_status"`
}

// pp:data-source local
func newNovelFleetRebootPendingCmd(flags *rootFlags) *cobra.Command {
	var dbPath, orgFilter string
	var limit int

	cmd := &cobra.Command{
		Use:         "reboot-pending",
		Short:       "List endpoints fleet-wide where an installed update is waiting on a reboot to finish.",
		Annotations: map[string]string{"mcp:read-only": "true"},
		Long: `The action queue that closes out a patch cycle. Scans locally synced
endpoints across every organization and lists those reporting
reboot_required, ranked by remaining critical updates. Reboot state is buried
per-endpoint per-org in the Action1 API; this is one fleet-wide list.

Use this command for the concrete list of machines waiting on a reboot to
finish patching. Do NOT use it for a composite ranking; use
'fleet health-score' instead.`,
		Example: `  action1-cli fleet reboot-pending --agent
  action1-cli fleet reboot-pending --org <org-uuid> --limit 25`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			db, err := fleetOpenStore(cmd.Context(), dbPath)
			if err != nil {
				return err
			}
			defer db.Close()
			maybeEmitSyncHints(cmd, db, "endpoints", flags.maxAge)

			endpoints, err := fleetLoadAll(cmd.Context(), db, "endpoints")
			if err != nil {
				return err
			}

			rows := make([]rebootPendingRow, 0)
			for _, e := range endpoints {
				org := fleetOrgID(e)
				if orgFilter != "" && org != orgFilter {
					continue
				}
				if !fleetBoolish(fleetField(e, "reboot_required")) {
					continue
				}
				var crit int
				if mu := fleetNested(e, "missing_updates"); mu != nil {
					c, _ := fleetNum(mu["critical"])
					crit = int(c)
				}
				name := fleetStrField(e, "name")
				if name == "" {
					name = fleetStrField(e, "device_name")
				}
				rows = append(rows, rebootPendingRow{
					OrganizationID: org,
					EndpointID:     fleetStrField(e, "id"),
					Name:           name,
					OS:             fleetStrField(e, "OS"),
					MissingCrit:    crit,
					LastSeen:       fleetStrField(e, "last_seen"),
					OnlineStatus:   fleetStrField(e, "online_status"),
				})
			}

			sort.SliceStable(rows, func(i, j int) bool {
				if rows[i].MissingCrit != rows[j].MissingCrit {
					return rows[i].MissingCrit > rows[j].MissingCrit
				}
				return rows[i].LastSeen > rows[j].LastSeen
			})
			if limit > 0 && len(rows) > limit {
				rows = rows[:limit]
			}

			header := []string{"ORG", "ENDPOINT", "NAME", "OS", "CRIT_REMAINING", "LAST_SEEN", "STATUS"}
			matrix := make([][]string, 0, len(rows))
			for _, r := range rows {
				matrix = append(matrix, []string{r.OrganizationID, r.EndpointID, r.Name, r.OS,
					fleetItoa(float64(r.MissingCrit)), r.LastSeen, r.OnlineStatus})
			}
			return fleetEmit(cmd, flags, rows, header, matrix)
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Local database path (default: standard cache location)")
	cmd.Flags().StringVar(&orgFilter, "org", fleetEnvOrg(), "Limit to one organization ID (default: $ACTION1_ORG_ID)")
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum endpoints to return (0 = all)")
	return cmd
}
