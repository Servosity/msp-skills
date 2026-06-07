// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

// pp:data-source local
package cli

import (
	"database/sql"
	"sort"

	"github.com/spf13/cobra"
	"tactical-rmm-pp-cli/internal/fleet"
)

// tAgentRow is the shared agent-shaped row used by triage, coverage and
// agents stale. Kept back-compatible with the prior 4.19 JSON shape.
type tAgentRow struct {
	AgentID  string   `json:"agent_id"`
	Hostname string   `json:"hostname"`
	Client   string   `json:"client"`
	Site     string   `json:"site"`
	Status   string   `json:"status"`
	Score    int      `json:"score,omitempty"`
	Reasons  []string `json:"reasons,omitempty"`
	LastSeen string   `json:"last_seen,omitempty"`
}

func newNovelTriageCmd(flags *rootFlags) *cobra.Command {
	var limit int
	cmd := &cobra.Command{
		Use:         "triage",
		Short:       "Rank agents that need attention (offline, failing checks, reboots, patches)",
		Long:        "Scores every synced agent across offline state, failing checks, pending reboots, patches and pending actions, then ranks them. Use this to rank individual agents by how much attention they need. To rank failing checks by blast radius instead, use 'checks worst'.",
		Example:     "  tactical-rmm-cli triage --limit 25\n  tactical-rmm-cli triage --limit 5 --json",
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
				q := "SELECT " + baseAgentCols + ",COALESCE(json_extract(data,'$.checks.failing'),0),COALESCE(json_extract(data,'$.needs_reboot'),0),COALESCE(json_extract(data,'$.has_patches_pending'),0),COALESCE(json_extract(data,'$.pending_actions_count'),0) FROM resources WHERE resource_type='agents'"
				if rows, qe := s.DB().QueryContext(cmd.Context(), q); qe == nil {
					defer rows.Close()
					for rows.Next() {
						var aid, host, cl, st, status sql.NullString
						var fc, rb, pp, pa sql.NullInt64
						if rows.Scan(&aid, &host, &cl, &st, &status, &fc, &rb, &pp, &pa) == nil {
							sig := fleet.AgentSignals{Status: status.String, FailingChecks: int(fc.Int64), NeedsReboot: rb.Int64 != 0, PatchesPending: pp.Int64 != 0, PendingActions: int(pa.Int64)}
							sc := fleet.TriageScore(sig)
							if sc <= 0 {
								continue
							}
							out = append(out, tAgentRow{AgentID: aid.String, Hostname: host.String, Client: cl.String, Site: st.String, Status: status.String, Score: sc, Reasons: fleet.Reasons(sig)})
						}
					}
				}
			}
			sort.Slice(out, func(i, j int) bool { return out[i].Score > out[j].Score })
			if limit > 0 && len(out) > limit {
				out = out[:limit]
			}
			return flags.printJSON(cmd, out)
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 25, "Max agents to return")
	return cmd
}
