// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel feature. Fleet alert-profile rollup: the alerting rules
// configured across the fleet, aggregated from the local store. The API exposes
// alert profiles and their bindings piecemeal.

package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// fleetAlertsQuery reads the documented AlertProfile fields (GET /alert-profile:
// id, name, is_enabled, description, tag). domotz handfix
// fleet-alerts-documented-fields: the profile id is the record's id (the
// alert_profile_id column is filled only by binding records), and no device
// is printed: a profile record names none, so the column was always null.
const fleetAlertsQuery = `
SELECT
  COALESCE(alert_profile_id, CAST(id AS INTEGER))         AS alert_profile_id,
  name                                                    AS name,
  description                                             AS description,
  CASE WHEN is_enabled = 1 THEN 1 ELSE 0 END              AS enabled,
  tag                                                     AS tag
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
			"view, enabled first. Reads the local store; run " +
			"'domotz-cli sync --resources alert-profile' (or 'sync') first. Which devices each " +
			"profile watches is not in the profile list; 'fleet unalerted' reports the devices no profile watches.",
		Example:     "  domotz-cli fleet alerts --json",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			db, err := novelOpenStoreChecked(cmd, flags, dbPath, "alert-profile", "alert-profile")
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
