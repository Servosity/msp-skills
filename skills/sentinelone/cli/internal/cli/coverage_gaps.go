// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

type coverageGap struct {
	ID           string   `json:"id"`
	ComputerName string   `json:"computer_name"`
	Site         string   `json:"site"`
	Group        string   `json:"group,omitempty"`
	Gaps         []string `json:"gaps"`
}

// newNovelCoverageGapsCmd lists endpoints with protection holes — detect-only
// mode, Ranger disabled, or firewall control disabled — grouped by site. It
// only flags fields that are actually present, so an unknown value is never
// reported as a false gap.
// pp:data-source local
func newNovelCoverageGapsCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	var limit int

	cmd := &cobra.Command{
		Use:   "gaps",
		Short: "List endpoints with protection holes (detect-only, Ranger off, firewall off), by site",
		Long: `List the specific endpoints with a protection gap:

  detect-only      mitigation mode is "detect", not "protect"
  ranger-disabled  Ranger (network visibility) is off on the agent
  firewall-off     agent firewall control is disabled

Only fields actually present on the agent are evaluated, so a missing value is
never reported as a false gap. Decommissioned agents are excluded.`,
		Example: `  # All endpoints with protection gaps
  sentinelone-cli coverage gaps

  # JSON for scripting
  sentinelone-cli coverage gaps --agent`,
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			db, err := openS1Store(cmd, dbPath)
			if err != nil {
				return err
			}
			defer db.Close()

			if !hintIfUnsynced(cmd, db, "agents") {
				hintIfStale(cmd, db, "agents", flags.maxAge)
			}

			agents, err := loadAgents(cmd.Context(), db)
			if err != nil {
				return fmt.Errorf("loading agents: %w", err)
			}
			if len(agents) == 0 {
				return honestEmptyJSON(cmd, flags,
					"No agents in the local store. Run 'sentinelone-cli sync' first.", nil)
			}

			var gaps []coverageGap
			for _, a := range agents {
				if agentDecommissioned(a) {
					continue
				}
				var g []string
				if mode := gstr(a, "mitigationMode"); mode != "" && !strings.EqualFold(mode, "protect") {
					g = append(g, "detect-only")
				}
				if rs := gstr(a, "rangerStatus"); rs != "" && !strings.EqualFold(rs, "Enabled") {
					g = append(g, "ranger-disabled")
				}
				if _, ok := gval(a, "firewallEnabled"); ok && !gbool(a, "firewallEnabled") {
					g = append(g, "firewall-off")
				}
				if len(g) == 0 {
					continue
				}
				gaps = append(gaps, coverageGap{
					ID:           gstrFirst(a, "id", "uuid"),
					ComputerName: gstrFirst(a, "computerName", "id"),
					Site:         orUnknown(agentSite(a)),
					Group:        gstr(a, "groupName"),
					Gaps:         g,
				})
			}

			// Group by site for the human view; sort sites by gap count desc.
			sort.SliceStable(gaps, func(i, j int) bool {
				if gaps[i].Site != gaps[j].Site {
					return gaps[i].Site < gaps[j].Site
				}
				return gaps[i].ComputerName < gaps[j].ComputerName
			})
			total := len(gaps)
			shown := gaps
			if limit > 0 && len(shown) > limit {
				shown = shown[:limit]
			}

			if total == 0 {
				return honestEmptyJSON(cmd, flags,
					"No protection gaps found — every non-decommissioned agent is in Protect mode with Ranger and firewall intact (for the fields present).", nil)
			}

			if flags.asJSON {
				return flags.printJSON(cmd, map[string]any{
					"total_gaps": total,
					"showing":    len(shown),
					"endpoints":  shown,
				})
			}
			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "%d endpoints with protection gaps:\n\n", total)
			fmt.Fprintf(w, "%-28s %-22s %s\n", "ENDPOINT", "SITE", "GAPS")
			for _, g := range shown {
				fmt.Fprintf(w, "%-28s %-22s %s\n", clip(g.ComputerName, 28), clip(g.Site, 22), strings.Join(g.Gaps, ", "))
			}
			if len(shown) < total {
				fmt.Fprintf(w, "\n(showing %d of %d; use --limit 0 for all)\n", len(shown), total)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/sentinelone-cli/data.db)")
	cmd.Flags().IntVar(&limit, "limit", 100, "Maximum endpoints to show (0 = all)")
	return cmd
}
