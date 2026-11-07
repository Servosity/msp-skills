// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// pp:data-source live
package cli

import (
	"sort"

	"github.com/spf13/cobra"
)

// Data-source strategy: a check's status lives on each agent's CheckResult,
// which the synced "checks" resource (GET /checks/) does not carry (see
// checks_live.go). This command takes the agents whose synced row reports a
// failing check of any alert severity (checks.failing, warning or info), fans out live to GET
// /agents/{agent_id}/checks/ for each (capped by --max-scan-agents), and counts
// the distinct agents whose result for each check is "failing". A check with
// no result is unknown, never failing. The JSON envelope reports
// candidate_agents, scanned_agents and fetch_failures.

func newNovelChecksWorstCmd(flags *rootFlags) *cobra.Command {
	var limit int
	var maxScan int
	cmd := &cobra.Command{
		Use:   "worst",
		Short: "Failing checks ranked by blast radius (agents affected)",
		Long: `Use this command to rank failing checks by how many agents they affect. Do NOT
use it to rank individual agents; use 'triage' instead.

A check's status is kept per agent, so this reads each agent's checks live:
it takes the synced agents that report failing checks, calls GET
/agents/{agent_id}/checks/ for each (capped by --max-scan-agents), groups the
results by check name and type, counts the agents whose result is failing,
and ranks descending. A check that has not run yet is counted as unknown,
never as failing. Run 'sync' first so the agent list is current.`,
		Example:     "  tactical-rmm-cli checks worst\n  tactical-rmm-cli checks worst --limit 5 --max-scan-agents 200 --json",
		Annotations: tRO(),
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "live"); err != nil {
				return err
			}
			type item struct {
				Check         string   `json:"check"`
				Type          string   `json:"check_type"`
				AgentsFailing int      `json:"agents_failing"`
				Agents        []string `json:"agents"`
			}
			items := make([]item, 0)
			envelope := map[string]any{"items": items, "candidate_agents": 0, "scanned_agents": 0, "fetch_failures": 0, "status_unknown": 0}

			refs := loadTrmmAgentRefs(cmd, flags, true)
			envelope["candidate_agents"] = len(refs)
			if len(refs) == 0 {
				envelope["note"] = "no synced agent reports failing checks; run 'tactical-rmm-cli sync' to refresh the agent list"
				return flags.printJSON(cmd, envelope)
			}
			if maxScan > 0 && len(refs) > maxScan {
				refs = refs[:maxScan]
				envelope["note"] = "scan capped by --max-scan-agents; agents with the most failing checks were scanned first"
			}
			c, err := flags.newClient()
			if err != nil {
				envelope["note"] = "live fan-out unavailable (no API client configured)"
				_ = flags.printJSON(cmd, envelope)
				return err
			}
			scan := fetchAgentChecksLive(cmd.Context(), c, refs)

			type key struct{ name, ctype string }
			failing := map[key]map[string]string{} // check -> agent_id -> display name
			for _, r := range scan.Rows {
				if r.Status != "failing" {
					continue
				}
				k := key{r.Name, r.CheckType}
				if failing[k] == nil {
					failing[k] = map[string]string{}
				}
				disp := r.Hostname
				if disp == "" {
					disp = r.AgentID
				}
				failing[k][r.AgentID] = disp
			}
			for k, agents := range failing {
				names := make([]string, 0, len(agents))
				for _, d := range agents {
					names = append(names, d)
				}
				sort.Strings(names)
				items = append(items, item{Check: k.name, Type: k.ctype, AgentsFailing: len(agents), Agents: names})
			}
			sort.Slice(items, func(i, j int) bool {
				if items[i].AgentsFailing != items[j].AgentsFailing {
					return items[i].AgentsFailing > items[j].AgentsFailing
				}
				if items[i].Check != items[j].Check {
					return items[i].Check < items[j].Check
				}
				return items[i].Type < items[j].Type
			})
			if limit > 0 && len(items) > limit {
				items = items[:limit]
			}
			envelope["items"] = items
			reportCheckScan(envelope, scan)
			if err := flags.printJSON(cmd, envelope); err != nil {
				return err
			}
			return checkScanOutcome(cmd, scan)
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 10, "Max checks to return")
	cmd.Flags().IntVar(&maxScan, "max-scan-agents", 50, "Max agents with failing checks to fan out to live")
	return cmd
}
