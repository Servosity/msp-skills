// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// pp:data-source local
package cli

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

// Data-source strategy: the report (no --snapshot) reads only the local
// check_snapshots history. --snapshot records it, and a check's status lives
// on each agent's CheckResult, which the synced "checks" resource (GET
// /checks/) does not carry (see checks_live.go). So --snapshot fans out live
// to GET /agents/{agent_id}/checks/ for every synced agent (capped by
// --max-scan-agents) and stores each check's real status, skipping checks with
// no result rather than storing an empty status.

func newNovelChecksFlappingCmd(flags *rootFlags) *cobra.Command {
	var window string
	var minFlips int
	var snapshot bool
	var maxScan int
	cmd := &cobra.Command{
		Use:         "flapping",
		Short:       "Checks that repeatedly changed pass/fail (from local snapshots)",
		Long:        "Maintains a local history of check statuses. Run with --snapshot periodically to record state: it reads each synced agent's checks live (GET /agents/{agent_id}/checks/, capped by --max-scan-agents) and stores every check's status, skipping checks that have not run. Without --snapshot it reads only the local history and reports checks that flipped at least --min-flips times within --window.",
		Example:     "  tactical-rmm-cli checks flapping --snapshot\n  tactical-rmm-cli checks flapping --window 24h --json",
		Annotations: tRO(),
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			strategy := "local"
			if snapshot {
				strategy = "live"
			}
			if err := validateDataSourceStrategy(flags, strategy); err != nil {
				return err
			}
			if snapshot {
				return recordCheckSnapshot(cmd, flags, maxScan)
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
	cmd.Flags().BoolVar(&snapshot, "snapshot", false, "Record each synced agent's current check statuses (read live) to local history")
	cmd.Flags().IntVar(&maxScan, "max-scan-agents", 50, "With --snapshot: max agents to fan out to live")
	return cmd
}

// recordCheckSnapshot stores each scanned agent's real check statuses in
// check_snapshots. Checks with no result are skipped, never stored as "".
func recordCheckSnapshot(cmd *cobra.Command, flags *rootFlags, maxScan int) error {
	envelope := map[string]any{"snapshot_recorded": 0, "candidate_agents": 0, "scanned_agents": 0, "fetch_failures": 0, "status_unknown": 0}
	refs := loadTrmmAgentRefs(cmd, flags, false)
	envelope["candidate_agents"] = len(refs)
	if len(refs) == 0 {
		envelope["note"] = "no agents in local store; run 'tactical-rmm-cli sync' first"
		return flags.printJSON(cmd, envelope)
	}
	if maxScan > 0 && len(refs) > maxScan {
		refs = refs[:maxScan]
		envelope["note"] = "scan capped by --max-scan-agents"
	}
	c, err := flags.newClient()
	if err != nil {
		envelope["note"] = "live fan-out unavailable (no API client configured)"
		_ = flags.printJSON(cmd, envelope)
		return err
	}
	scan := fetchAgentChecksLive(cmd.Context(), c, refs)
	s, err := tOpen(cmd.Context())
	if err != nil {
		return fmt.Errorf("open local store: %w", err)
	}
	defer s.Close()
	db := s.DB()
	ctx := cmd.Context()
	if _, err := db.ExecContext(ctx, "CREATE TABLE IF NOT EXISTS check_snapshots (check_id TEXT, agent_id TEXT, status TEXT, ts TIMESTAMP DEFAULT CURRENT_TIMESTAMP)"); err != nil {
		return fmt.Errorf("create check_snapshots table: %w", err)
	}
	n := 0
	for _, r := range scan.Rows {
		if r.Status == checkStatusUnknown || r.CheckID == "" {
			continue
		}
		if _, err := db.ExecContext(ctx, "INSERT INTO check_snapshots(check_id,agent_id,status) VALUES(?,?,?)", r.CheckID, r.AgentID, r.Status); err != nil {
			return fmt.Errorf("record check snapshot: %w", err)
		}
		n++
	}
	envelope["snapshot_recorded"] = n
	reportCheckScan(envelope, scan)
	if err := flags.printJSON(cmd, envelope); err != nil {
		return err
	}
	return checkScanOutcome(cmd, scan)
}
