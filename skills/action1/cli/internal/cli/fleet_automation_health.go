// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

type automationHealthResult struct {
	TotalInstances    int            `json:"total_instances"`
	Succeeded         int            `json:"succeeded"`
	Failed            int            `json:"failed"`
	Running           int            `json:"running"`
	Other             int            `json:"other"`
	SuccessRate       float64        `json:"success_rate"`
	InstancesByStatus map[string]int `json:"instances_by_status"`
}

// pp:data-source local
func newNovelFleetAutomationHealthCmd(flags *rootFlags) *cobra.Command {
	var dbPath, orgFilter string

	cmd := &cobra.Command{
		Use:         "automation-health",
		Short:       "Aggregate automation success/failure rates across all organizations.",
		Annotations: map[string]string{"mcp:read-only": "true"},
		Long: `Fleet-wide automation outcomes. Reads locally synced automation instances
from every organization and rolls up their statuses into succeeded / failed /
running counts and an overall success rate. Action1 reports automation
instances one organization at a time; this is the cross-org rollup the API
does not provide.

Run 'action1-cli sync' (which fans automation instances out across every
organization) before this command.`,
		Example: `  action1-cli fleet automation-health --agent
  action1-cli fleet automation-health --org <org-uuid>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			db, err := fleetOpenStore(cmd.Context(), dbPath)
			if err != nil {
				return err
			}
			defer db.Close()
			maybeEmitSyncHints(cmd, db, "automation_instances", flags.maxAge)

			instances, err := fleetLoadAll(cmd.Context(), db, "automation_instances")
			if err != nil {
				return err
			}

			result := automationHealthResult{InstancesByStatus: map[string]int{}}
			for _, inst := range instances {
				if orgFilter != "" && fleetOrgID(inst) != orgFilter {
					continue
				}
				status := fleetStrField(inst, "status")
				if status == "" {
					status = "Unknown"
				}
				result.TotalInstances++
				result.InstancesByStatus[status]++
				switch classifyAutomationStatus(status) {
				case "succeeded":
					result.Succeeded++
				case "failed":
					result.Failed++
				case "running":
					result.Running++
				default:
					result.Other++
				}
			}
			if denom := result.Succeeded + result.Failed; denom > 0 {
				result.SuccessRate = round1(float64(result.Succeeded) / float64(denom) * 100)
			}

			// Human table: status -> count, sorted by count desc.
			type kv struct {
				k string
				v int
			}
			pairs := make([]kv, 0, len(result.InstancesByStatus))
			for k, v := range result.InstancesByStatus {
				pairs = append(pairs, kv{k, v})
			}
			sort.SliceStable(pairs, func(i, j int) bool { return pairs[i].v > pairs[j].v })
			header := []string{"STATUS", "COUNT"}
			matrix := make([][]string, 0, len(pairs))
			for _, p := range pairs {
				matrix = append(matrix, []string{p.k, fleetItoa(float64(p.v))})
			}
			matrix = append(matrix, []string{"success_rate%", fmtFloat(result.SuccessRate)})
			return fleetEmit(cmd, flags, result, header, matrix)
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Local database path (default: standard cache location)")
	cmd.Flags().StringVar(&orgFilter, "org", fleetEnvOrg(), "Limit to one organization ID (default: $ACTION1_ORG_ID)")
	return cmd
}

// classifyAutomationStatus buckets an Action1 automation status string.
func classifyAutomationStatus(status string) string {
	s := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(status, "_", ""), " ", ""))
	switch {
	case strings.Contains(s, "complet"), strings.Contains(s, "success"), strings.Contains(s, "succeeded"),
		s == "finished", s == "done", s == "ok":
		return "succeeded"
	case strings.Contains(s, "fail"), strings.Contains(s, "error"), strings.Contains(s, "fault"):
		return "failed"
	case strings.Contains(s, "running"), strings.Contains(s, "inprogress"), strings.Contains(s, "pending"),
		strings.Contains(s, "scheduled"), strings.Contains(s, "started"), strings.Contains(s, "queued"), strings.Contains(s, "active"):
		return "running"
	}
	return "other"
}
