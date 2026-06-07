// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel feature. Fleet device-type/vendor/OS breakdown: counts
// across every device for capacity planning and security posture. The API has
// no cross-fleet analytics endpoint.

package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// breakdownFields maps the --by value to the device JSON path it groups on.
// Whitelisted to keep the column out of string-built SQL.
var breakdownFields = map[string]string{
	"type":   "$.type.label",
	"vendor": "$.user_data.vendor",
	"os":     "$.os.name",
}

// pp:data-source local
func newNovelFleetBreakdownCmd(flags *rootFlags) *cobra.Command {
	var flagBy string
	var dbPath string

	cmd := &cobra.Command{
		Use:   "breakdown",
		Short: "Count devices grouped by type, vendor, or OS across the fleet",
		Long: "Aggregate device counts across the whole fleet by type (default), vendor, or OS — " +
			"answers 'how many of X do we manage' across all clients at once. Reads the local " +
			"store; run 'domotz-cli sync --full' first.",
		Example:     "  domotz-cli fleet breakdown --by vendor --json",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			path, ok := breakdownFields[flagBy]
			if !ok {
				return usageErr(fmt.Errorf("invalid --by %q: must be one of type, vendor, os", flagBy))
			}
			db, err := novelOpenStoreChecked(cmd, flags, dbPath, "device")
			if err != nil {
				return err
			}
			defer db.Close()

			query := fmt.Sprintf(`
SELECT
  COALESCE(NULLIF(json_extract(data, '%s'), ''), '(unknown)') AS %s,
  COUNT(*) AS count
FROM "device"
GROUP BY 1
ORDER BY count DESC, 1`, path, flagBy)

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
	cmd.Flags().StringVar(&flagBy, "by", "type", "Group devices by: type, vendor, or os")
	addFleetDBFlag(cmd, &dbPath)
	return cmd
}
