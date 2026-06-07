// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

// pp:data-source local
package cli

import (
	"database/sql"

	"github.com/spf13/cobra"
)

func newNovelChecksWorstCmd(flags *rootFlags) *cobra.Command {
	var limit int
	cmd := &cobra.Command{
		Use:   "worst",
		Short: "Failing checks ranked by blast radius (agents affected)",
		Long: `Use this command to rank failing checks by how many agents they affect. Do NOT
use it to rank individual agents; use 'triage' instead.

Groups synced check rows by check name/type, counts the agents currently
failing each, and ranks descending. Reads only the local store.`,
		Example:     "  tactical-rmm-cli checks worst\n  tactical-rmm-cli checks worst --limit 5 --json",
		Annotations: tRO(),
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			type row struct {
				Check        string `json:"check"`
				Type         string `json:"check_type"`
				AgentsRed    int    `json:"agents_failing"`
				TotalRunning int    `json:"total_checks"`
			}
			out := make([]row, 0)
			if s := novelLocalRead(cmd, flags, "checks"); s != nil {
				defer s.Close()
				q := `SELECT
					COALESCE(json_extract(data,'$.readable_desc'),json_extract(data,'$.name'),json_extract(data,'$.check_type'),'(unnamed)') AS cname,
					COALESCE(json_extract(data,'$.check_type'),'(unknown)') AS ctype,
					SUM(CASE WHEN lower(COALESCE(json_extract(data,'$.status'),''))='failing' THEN 1 ELSE 0 END) AS red,
					COUNT(*) AS total
				FROM resources WHERE resource_type='checks'
				GROUP BY cname, ctype
				HAVING red > 0
				ORDER BY red DESC, total DESC`
				if rows, qe := s.DB().QueryContext(cmd.Context(), q); qe == nil {
					defer rows.Close()
					for rows.Next() {
						var cn, ct sql.NullString
						var red, total sql.NullInt64
						if rows.Scan(&cn, &ct, &red, &total) == nil {
							out = append(out, row{Check: cn.String, Type: ct.String, AgentsRed: int(red.Int64), TotalRunning: int(total.Int64)})
						}
					}
				}
			}
			if limit > 0 && len(out) > limit {
				out = out[:limit]
			}
			return flags.printJSON(cmd, out)
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 10, "Max checks to return")
	return cmd
}
