// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Novel feature (hand-built): alerting coverage gaps via monitors×policies anti-join.

package cli

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"
)

type coverageGap struct {
	ID     string   `json:"id"`
	Name   string   `json:"name"`
	URL    string   `json:"url"`
	Status string   `json:"status"`
	Paused bool     `json:"paused"`
	Why    []string `json:"why"`
}

// pp:data-source local
func newNovelCoverageCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	var includePaused bool

	cmd := &cobra.Command{
		Use:         "coverage",
		Short:       "Find monitors with no escalation policy or no alert channel — the ones that will go down silently and page no one.",
		Long:        "Use this command to find monitors with no escalation policy or alert channel. Do NOT use it for heartbeat configuration risk; use 'heartbeat-risk' instead. Do NOT use it to find rotations with nobody on call; use 'oncall-gaps' instead. Anti-joins monitors against escalation policies and direct alert channels in the local mirror to surface monitors that would page nobody if they failed. Run `sync` first.",
		Example:     "  betterstack-cli coverage\n  betterstack-cli coverage --agent\n  betterstack-cli coverage --include-paused",
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
			maybeEmitSyncHints(cmd, s, "monitors", flags.maxAge)

			monitors, err := loadMonitors(cmd.Context(), s)
			if err != nil {
				return fmt.Errorf("reading monitors: %w", err)
			}

			gaps := make([]coverageGap, 0)
			for _, m := range monitors {
				if m.Paused && !includePaused {
					continue
				}
				var why []string
				if m.PolicyID == "" {
					why = append(why, "no escalation policy")
				}
				if !m.hasAlertChannel() {
					why = append(why, "no alert channel (email/sms/call/push all off)")
				}
				if len(why) == 0 {
					continue
				}
				name := m.Name
				if name == "" {
					name = m.URL
				}
				gaps = append(gaps, coverageGap{ID: m.ID, Name: name, URL: m.URL, Status: m.Status, Paused: m.Paused, Why: why})
			}
			// Worst first: monitors missing BOTH policy and channel, then by name.
			sort.SliceStable(gaps, func(i, j int) bool {
				if len(gaps[i].Why) != len(gaps[j].Why) {
					return len(gaps[i].Why) > len(gaps[j].Why)
				}
				return gaps[i].Name < gaps[j].Name
			})

			if flags.asJSON {
				return flags.printJSON(cmd, gaps)
			}
			if len(gaps) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No coverage gaps: every monitor has an escalation policy or an alert channel.")
				return nil
			}
			rows := make([][]string, 0, len(gaps))
			for _, g := range gaps {
				reason := ""
				for i, w := range g.Why {
					if i > 0 {
						reason += "; "
					}
					reason += w
				}
				rows = append(rows, []string{g.ID, truncateField(g.Name, 40), g.Status, reason})
			}
			return flags.printTable(cmd, []string{"ID", "MONITOR", "STATUS", "GAP"}, rows)
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/betterstack-cli/data.db)")
	cmd.Flags().BoolVar(&includePaused, "include-paused", false, "Include paused monitors in the gap analysis")
	return cmd
}

func truncateField(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n <= 1 {
		return string(r[:n])
	}
	return string(r[:n-1]) + "…"
}
