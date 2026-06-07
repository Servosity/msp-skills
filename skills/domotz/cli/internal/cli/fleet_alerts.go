// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel feature. Fleet alert-profile rollup: the alerting rules
// configured across the fleet, aggregated from the local store. The API exposes
// alert profiles and their bindings piecemeal.

package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// fleetAlertsQuery lists alert profiles, enabled first, with their bound device.
const fleetAlertsQuery = `
SELECT
  alert_profile_id                                        AS alert_profile_id,
  name                                                    AS name,
  description                                             AS description,
  CASE WHEN is_enabled = 1 THEN 1 ELSE 0 END              AS enabled,
  tag                                                     AS tag,
  device_id                                               AS device_id
FROM "alert_profile"
%s
ORDER BY enabled DESC, name`

// pp:data-source local
func newNovelFleetAlertsCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	var enabledOnly bool

	cmd := &cobra.Command{
		Use:   "alerts",
		Short: "Alert profiles configured across the fleet, enabled first",
		Long: "Aggregate the alert profiles (alerting rules) configured across the fleet into one " +
			"view, enabled first, with their bound device. Reads the local store; run " +
			"'domotz-cli sync --resources alert_profile' (or 'sync --full') first.",
		Example:     "  domotz-cli fleet alerts --json",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			db, err := novelOpenStoreChecked(cmd, flags, dbPath, "alert-profile")
			if err != nil {
				return err
			}
			defer db.Close()

			where := ""
			if enabledOnly {
				where = "WHERE is_enabled = 1"
			}
			query := fmt.Sprintf(fleetAlertsQuery, where)

			rows, err := queryFleetRows(cmd.Context(), db, query)
			if err != nil {
				return err
			}
			if wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printAutoTable(cmd.OutOrStdout(), rows)
			}
			return printJSONFiltered(cmd.OutOrStdout(), rows, flags)
		},
	}
	cmd.Flags().BoolVar(&enabledOnly, "enabled-only", false, "Show only enabled alert profiles")
	addFleetDBFlag(cmd, &dbPath)
	return cmd
}
