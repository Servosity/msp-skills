// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

// pp:data-source local
package cli

import (
	"database/sql"
	"time"

	"github.com/spf13/cobra"
)

func newNovelAlertsDigestCmd(flags *rootFlags) *cobra.Command {
	var since, by string
	cmd := &cobra.Command{
		Use:         "digest",
		Short:       "Grouped alert summary over a time window",
		Long:        "Groups alerts within --since by --by (severity, client, or type) and reports total and still-active counts per group. Reads only the local store.",
		Example:     "  tactical-rmm-cli alerts digest --since 24h --by severity\n  tactical-rmm-cli alerts digest --since 7d --by client --json",
		Annotations: tRO(),
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			field := "severity"
			switch by {
			case "client":
				field = "client"
			case "type":
				field = "alert_type"
			}
			cutoff := time.Now().Add(-tWindow(since)).UTC().Format("2006-01-02 15:04:05")
			type row struct {
				Group  string `json:"group"`
				Total  int    `json:"total"`
				Active int    `json:"active"`
			}
			out := make([]row, 0)
			if s := novelLocalRead(cmd, flags, "alerts"); s != nil {
				defer s.Close()
				q := "SELECT COALESCE(json_extract(data,'$." + field + "'),'(none)') g, COUNT(*), SUM(CASE WHEN COALESCE(json_extract(data,'$.resolved'),0)=0 THEN 1 ELSE 0 END) FROM resources WHERE resource_type='alerts' AND " + sqlISOToDatetime("alert_time") + ">=datetime(?) GROUP BY g ORDER BY 2 DESC"
				if rows, qe := s.DB().QueryContext(cmd.Context(), q, cutoff); qe == nil {
					defer rows.Close()
					for rows.Next() {
						var g sql.NullString
						var t, a sql.NullInt64
						if rows.Scan(&g, &t, &a) == nil {
							out = append(out, row{g.String, int(t.Int64), int(a.Int64)})
						}
					}
				}
			}
			return flags.printJSON(cmd, out)
		},
	}
	cmd.Flags().StringVar(&since, "since", "24h", "Time window (e.g. 2h, 24h, 7d)")
	cmd.Flags().StringVar(&by, "by", "severity", "Group by: severity, client, or type")
	return cmd
}
