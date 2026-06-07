// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

// pp:data-source local
package cli

import (
	"github.com/spf13/cobra"
)

func newNovelCoverageCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "coverage",
		Short:       "List agents with no checks configured (monitoring gaps)",
		Long:        "Lists every synced agent that has zero checks configured, so unmonitored endpoints surface. Reads only the local store.",
		Example:     "  tactical-rmm-cli coverage\n  tactical-rmm-cli coverage --json",
		Annotations: tRO(),
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
				q := "SELECT " + baseAgentCols + " FROM resources WHERE resource_type='agents' AND COALESCE(json_extract(data,'$.checks.total'),0)=0"
				if rows, qe := s.DB().QueryContext(cmd.Context(), q); qe == nil {
					defer rows.Close()
					for rows.Next() {
						if row, ok := scanBaseAgent(rows); ok {
							row.Reasons = []string{"no checks configured"}
							out = append(out, row)
						}
					}
				}
			}
			return flags.printJSON(cmd, out)
		},
	}
	return cmd
}
