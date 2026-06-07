// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"sort"

	"github.com/spf13/cobra"
)

type patchPostureRow struct {
	OrganizationID string `json:"organization_id"`
	EndpointID     string `json:"endpoint_id"`
	Name           string `json:"name"`
	OS             string `json:"os"`
	Critical       int    `json:"critical_updates"`
	Other          int    `json:"other_updates"`
	TotalMissing   int    `json:"total_missing"`
	LastSeen       string `json:"last_seen"`
}

// pp:data-source local
func newNovelFleetPatchPostureCmd(flags *rootFlags) *cobra.Command {
	var dbPath, orgFilter string
	var limit int

	cmd := &cobra.Command{
		Use:         "patch-posture",
		Short:       "Rank endpoints across all organizations by how many updates they are missing.",
		Annotations: map[string]string{"mcp:read-only": "true"},
		Long: `Fleet-wide patch posture. Reads locally synced endpoints across every
organization and ranks them by missing-update count (critical first), a view
the org-scoped Action1 API does not provide.

Missing-update counts come from the endpoint's missing_updates summary, which
Action1 returns only when the endpoint was synced with fields including
missing_updates. Endpoints without the summary are reported with zero counts.`,
		Example: `  action1-cli fleet patch-posture --agent --limit 25
  action1-cli fleet patch-posture --org <org-uuid>`,
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

			rows := make([]patchPostureRow, 0, len(endpoints))
			for _, e := range endpoints {
				org := fleetOrgID(e)
				if orgFilter != "" && org != orgFilter {
					continue
				}
				mu := fleetNested(e, "missing_updates")
				var crit, other int
				if mu != nil {
					c, _ := fleetNum(mu["critical"])
					o, _ := fleetNum(mu["other"])
					crit, other = int(c), int(o)
				}
				name := fleetStrField(e, "name")
				if name == "" {
					name = fleetStrField(e, "device_name")
				}
				rows = append(rows, patchPostureRow{
					OrganizationID: org,
					EndpointID:     fleetStrField(e, "id"),
					Name:           name,
					OS:             fleetStrField(e, "OS"),
					Critical:       crit,
					Other:          other,
					TotalMissing:   crit + other,
					LastSeen:       fleetStrField(e, "last_seen"),
				})
			}

			sort.SliceStable(rows, func(i, j int) bool {
				if rows[i].Critical != rows[j].Critical {
					return rows[i].Critical > rows[j].Critical
				}
				return rows[i].TotalMissing > rows[j].TotalMissing
			})
			if limit > 0 && len(rows) > limit {
				rows = rows[:limit]
			}

			header := []string{"ORG", "ENDPOINT", "NAME", "OS", "CRITICAL", "OTHER", "TOTAL", "LAST_SEEN"}
			matrix := make([][]string, 0, len(rows))
			for _, r := range rows {
				matrix = append(matrix, []string{r.OrganizationID, r.EndpointID, r.Name, r.OS,
					fleetItoa(float64(r.Critical)), fleetItoa(float64(r.Other)), fleetItoa(float64(r.TotalMissing)), r.LastSeen})
			}
			return fleetEmit(cmd, flags, rows, header, matrix)
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Local database path (default: standard cache location)")
	cmd.Flags().StringVar(&orgFilter, "org", fleetEnvOrg(), "Limit to one organization ID (default: $ACTION1_ORG_ID)")
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum endpoints to return (0 = all)")
	return cmd
}
