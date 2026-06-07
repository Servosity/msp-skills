// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Novel feature (hand-built): heartbeat risk audit from the local mirror.

package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

type heartbeatRisk struct {
	ID     string   `json:"id"`
	Name   string   `json:"name"`
	Status string   `json:"status"`
	Period int      `json:"period_seconds"`
	Grace  int      `json:"grace_seconds"`
	Paused bool     `json:"paused"`
	Score  int      `json:"risk_score"`
	Why    []string `json:"why"`
}

// pp:data-source local
func newNovelHeartbeatRiskCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	var top int

	cmd := &cobra.Command{
		Use:         "heartbeat-risk",
		Short:       "Rank heartbeats by risk: tight period+grace windows, paused-but-expected, and non-up status.",
		Long:        "Use this command to rank fragile heartbeats (cron/scheduled-task check-ins). Do NOT use it for monitor escalation coverage; use 'coverage' instead. Audits every heartbeat in the local mirror and scores risk from tight grace windows, paused state, and non-up status. Run `sync` first.",
		Example:     "  betterstack-cli heartbeat-risk\n  betterstack-cli heartbeat-risk --top 10 --agent",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			if dryRunOK(flags) {
				return nil
			}
			s, err := openAnalyticsStore(cmd, dbPath)
			if err != nil {
				return err
			}
			defer s.Close()
			maybeEmitSyncHints(cmd, s, "heartbeats", flags.maxAge)

			heartbeats, err := loadHeartbeats(cmd.Context(), s)
			if err != nil {
				return fmt.Errorf("reading heartbeats: %w", err)
			}

			risks := make([]heartbeatRisk, 0)
			for _, h := range heartbeats {
				var why []string
				score := 0
				// Paused heartbeats won't alert even if the task stops checking in.
				if h.Paused {
					score += 3
					why = append(why, "paused (won't alert)")
				}
				// Tight grace relative to period leaves little slack for a late check-in.
				if h.Period > 0 && h.Grace >= 0 && h.Grace < h.Period {
					score += 2
					why = append(why, fmt.Sprintf("grace (%ds) < period (%ds)", h.Grace, h.Period))
				}
				// Zero grace = no slack at all.
				if h.Grace == 0 {
					score += 2
					why = append(why, "zero grace period")
				}
				// Not currently up.
				if h.Status != "" && !strings.EqualFold(h.Status, "up") && !strings.EqualFold(h.Status, "paused") {
					score += 2
					why = append(why, "status: "+h.Status)
				}
				if score == 0 {
					continue
				}
				name := h.Name
				if name == "" {
					name = h.ID
				}
				risks = append(risks, heartbeatRisk{
					ID: h.ID, Name: name, Status: h.Status, Period: h.Period, Grace: h.Grace,
					Paused: h.Paused, Score: score, Why: why,
				})
			}
			sort.SliceStable(risks, func(i, j int) bool {
				if risks[i].Score != risks[j].Score {
					return risks[i].Score > risks[j].Score
				}
				return risks[i].Name < risks[j].Name
			})
			if top > 0 && len(risks) > top {
				risks = risks[:top]
			}

			if flags.asJSON {
				return flags.printJSON(cmd, risks)
			}
			if len(heartbeats) == 0 {
				fmt.Fprintln(cmd.ErrOrStderr(), "local mirror has no heartbeats — run `betterstack-cli sync` first")
			}
			if len(risks) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No at-risk heartbeats: all have sane grace windows and are active.")
				return nil
			}
			rows := make([][]string, 0, len(risks))
			for _, r := range risks {
				rows = append(rows, []string{
					r.ID, truncateField(r.Name, 36), fmt.Sprintf("%d", r.Score), strings.Join(r.Why, "; "),
				})
			}
			return flags.printTable(cmd, []string{"ID", "HEARTBEAT", "RISK", "WHY"}, rows)
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/betterstack-cli/data.db)")
	cmd.Flags().IntVar(&top, "top", 0, "Show only the top N riskiest heartbeats (0 = all)")
	return cmd
}
