// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

// pp:data-source local
package cli

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

func newNovelChecksFlappingCmd(flags *rootFlags) *cobra.Command {
	var window string
	var minFlips int
	var snapshot bool
	cmd := &cobra.Command{
		Use:         "flapping",
		Short:       "Checks that repeatedly changed pass/fail (from local snapshots)",
		Long:        "Maintains a local history of check statuses. Run with --snapshot periodically to record state; without it, reports checks that flipped at least --min-flips times within --window. Reads only the local store.",
		Example:     "  tactical-rmm-cli checks flapping --snapshot\n  tactical-rmm-cli checks flapping --window 24h --json",
		Annotations: tRO(),
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			s := novelLocalRead(cmd, flags, "checks")
			if s == nil {
				return flags.printJSON(cmd, make([]interface{}, 0))
			}
			defer s.Close()
			db := s.DB()
			ctx := cmd.Context()
			if _, err := db.ExecContext(ctx, "CREATE TABLE IF NOT EXISTS check_snapshots (check_id TEXT, agent_id TEXT, status TEXT, ts TIMESTAMP DEFAULT CURRENT_TIMESTAMP)"); err != nil {
				return fmt.Errorf("create check_snapshots table: %w", err)
			}
			if snapshot {
				n := 0
				if rows, qe := db.QueryContext(ctx, "SELECT json_extract(data,'$.id'),json_extract(data,'$.agent'),json_extract(data,'$.status') FROM resources WHERE resource_type='checks'"); qe == nil {
					for rows.Next() {
						var id, ag, stt sql.NullString
						if rows.Scan(&id, &ag, &stt) == nil {
							if _, ie := db.ExecContext(ctx, "INSERT INTO check_snapshots(check_id,agent_id,status) VALUES(?,?,?)", id.String, ag.String, stt.String); ie == nil {
								n++
							}
						}
					}
					_ = rows.Close()
				}
				return flags.printJSON(cmd, map[string]interface{}{"snapshot_recorded": n})
			}
			cutoff := time.Now().Add(-tWindow(window)).UTC().Format("2006-01-02 15:04:05")
			type row struct {
				CheckID string `json:"check_id"`
				AgentID string `json:"agent_id"`
				Flips   int    `json:"flips"`
			}
			out := make([]row, 0)
			q := "SELECT check_id, agent_id, (SELECT COUNT(*) FROM (SELECT status, LAG(status) OVER (ORDER BY ts) p FROM check_snapshots c2 WHERE c2.check_id=cs.check_id AND c2.agent_id=cs.agent_id AND c2.ts>=?) t WHERE p IS NOT NULL AND status<>p) flips FROM (SELECT DISTINCT check_id, agent_id FROM check_snapshots WHERE ts>=?) cs"
			if rows, qe := db.QueryContext(ctx, q, cutoff, cutoff); qe == nil {
				defer rows.Close()
				for rows.Next() {
					var cid, ag sql.NullString
					var fl sql.NullInt64
					if rows.Scan(&cid, &ag, &fl) == nil && int(fl.Int64) >= minFlips {
						out = append(out, row{cid.String, ag.String, int(fl.Int64)})
					}
				}
			}
			if len(out) == 0 {
				fmt.Fprintln(cmd.ErrOrStderr(), "note: no flapping detected (build history by running with --snapshot over time)")
			}
			return flags.printJSON(cmd, out)
		},
	}
	cmd.Flags().StringVar(&window, "window", "24h", "Time window (e.g. 2h, 24h, 7d)")
	cmd.Flags().IntVar(&minFlips, "min-flips", 3, "Minimum status changes to report")
	cmd.Flags().BoolVar(&snapshot, "snapshot", false, "Record current check statuses to local history")
	return cmd
}
