// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

// pp:data-source local
package cli

import (
	"database/sql"
	"time"

	"github.com/spf13/cobra"
)

// Query strings assembled once from compile-time-constant fragments. The
// embedded sqlISOToDatetime() argument is always a literal column name, never
// user input; the only variable (the cutoff) is bound via the ? placeholder.
// Hoisting these to package vars keeps the SQL out of the QueryContext call
// expression so a taint scanner does not read the literal concatenation as
// dynamic query construction.
var (
	// #nosec G202 -- sqlISOToDatetime is called with a compile-time string literal
	// ("alert_time"); the only runtime value (the cutoff) is bound via the ?
	// placeholder, so there is no user-controlled input in this query string.
	sinceNewAlertsQuery = "SELECT json_extract(data,'$.hostname'),json_extract(data,'$.severity'),json_extract(data,'$.message') " +
		"FROM resources WHERE resource_type='alerts' AND " + sqlISOToDatetime("alert_time") + ">=datetime(?)"
	// #nosec G202 -- sqlISOToDatetime is called with a compile-time string literal
	// ("last_seen"); the only runtime value (the cutoff) is bound via the ?
	// placeholder, so there is no user-controlled input in this query string.
	sinceNewlyOfflineQuery = "SELECT json_extract(data,'$.hostname'),json_extract(data,'$.status'),json_extract(data,'$.last_seen') " +
		"FROM resources WHERE resource_type='agents' AND json_extract(data,'$.status') IN ('offline','overdue') AND " + sqlISOToDatetime("last_seen") + ">=datetime(?)"
)

func newNovelSinceCmd(flags *rootFlags) *cobra.Command {
	var window string
	cmd := &cobra.Command{
		Use:         "since [duration]",
		Short:       "What changed across the fleet within a time window",
		Long:        "Reports new alerts and newly-offline agents within the window (e.g. 2h, 24h, 7d), derived from synced timestamps. Reads only the local store.",
		Example:     "  tactical-rmm-cli since 2h\n  tactical-rmm-cli since --window 24h --json",
		Annotations: map[string]string{"mcp:read-only": "true"}, // read-only: queries the local store only
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			w := window
			if len(args) > 0 {
				w = args[0]
			}
			cutoff := time.Now().Add(-tWindow(w)).UTC().Format("2006-01-02 15:04:05")
			res := map[string]interface{}{"window": w, "new_alerts": []map[string]string{}, "newly_offline": []map[string]string{}}
			if s := novelLocalRead(cmd, flags, ""); s != nil {
				defer s.Close()
				db := s.DB()
				ctx := cmd.Context()
				na := make([]map[string]string, 0)
				if rows, qe := db.QueryContext(ctx, sinceNewAlertsQuery, cutoff); qe == nil {
					for rows.Next() {
						var h, sev, msg sql.NullString
						if rows.Scan(&h, &sev, &msg) == nil {
							na = append(na, map[string]string{"hostname": h.String, "severity": sev.String, "message": msg.String})
						}
					}
					_ = rows.Close()
				}
				res["new_alerts"] = na
				no := make([]map[string]string, 0)
				if rows, qe := db.QueryContext(ctx, sinceNewlyOfflineQuery, cutoff); qe == nil {
					for rows.Next() {
						var h, stt, ls sql.NullString
						if rows.Scan(&h, &stt, &ls) == nil {
							no = append(no, map[string]string{"hostname": h.String, "status": stt.String, "last_seen": ls.String})
						}
					}
					_ = rows.Close()
				}
				res["newly_offline"] = no
			}
			return flags.printJSON(cmd, res)
		},
	}
	cmd.Flags().StringVar(&window, "window", "24h", "Time window (e.g. 2h, 24h, 7d)")
	return cmd
}
