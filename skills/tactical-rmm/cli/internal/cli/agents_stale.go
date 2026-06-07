// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

// pp:data-source local
package cli

import (
	"database/sql"

	"github.com/spf13/cobra"
)

func newNovelAgentsStaleCmd(flags *rootFlags) *cobra.Command {
	var days int
	cmd := &cobra.Command{
		Use:         "stale",
		Short:       "Agents whose last check-in exceeds a threshold",
		Long:        "Lists synced agents that are offline/overdue or whose last check-in is older than --days, with client and site. Reads only the local store.",
		Example:     "  tactical-rmm-cli agents stale --days 7\n  tactical-rmm-cli agents stale --days 14 --json",
		Annotations: map[string]string{"mcp:read-only": "true"}, // read-only: queries the local store only
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			out := make([]tAgentRow, 0)
			if s := novelLocalRead(cmd, flags, "agents"); s != nil {
				defer s.Close()
				q := "SELECT " + baseAgentCols + ",json_extract(data,'$.last_seen') FROM resources WHERE resource_type='agents' AND (json_extract(data,'$.status') IN ('offline','overdue') OR (julianday('now')-julianday(" + sqlISOToDatetime("last_seen") + "))>?) ORDER BY json_extract(data,'$.last_seen') ASC"
				if rows, qe := s.DB().QueryContext(cmd.Context(), q, days); qe == nil {
					defer rows.Close()
					for rows.Next() {
						var ls sql.NullString
						if row, ok := scanBaseAgent(rows, &ls); ok {
							row.LastSeen = ls.String
							out = append(out, row)
						}
					}
				}
			}
			return flags.printJSON(cmd, out)
		},
	}
	cmd.Flags().IntVar(&days, "days", 7, "Days since last check-in")
	return cmd
}
