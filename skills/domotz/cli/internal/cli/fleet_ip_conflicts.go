// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel feature. Fleet IP-conflict sweep: rolls up the per-agent
// ip-conflict endpoint across every agent into one prioritized list — a query
// the agent-scoped API cannot answer in a single call. Live fan-out across the
// agent list; the generated client enforces rate limiting.

package cli

import (
	"context"
	"domotz-pp-cli/internal/cliutil"
	"encoding/json"
	"fmt"
	"net/url"

	"github.com/spf13/cobra"
)

type ipConflictItem struct {
	IP                 string            `json:"ip"`
	ConflictingDevices []json.RawMessage `json:"conflicting_devices"`
}

type ipConflictRow struct {
	Site               string `json:"site"`
	AgentID            string `json:"agent_id"`
	IP                 string `json:"ip"`
	ConflictingDevices int    `json:"conflicting_devices"`
}

// parseIPConflicts tolerates both an array response and a single-object
// response from the ip-conflict endpoint.
func parseIPConflicts(data json.RawMessage) []ipConflictItem {
	var arr []ipConflictItem
	if err := json.Unmarshal(data, &arr); err == nil {
		return arr
	}
	var single ipConflictItem
	if err := json.Unmarshal(data, &single); err == nil && single.IP != "" {
		return []ipConflictItem{single}
	}
	return nil
}

// pp:data-source live
func newNovelFleetIpConflictsCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ip-conflicts",
		Short: "IP address conflicts across all sites in one list",
		Long: "Sweep every agent for IP address conflicts and roll them up into one list — the " +
			"per-agent ip-conflict endpoint, aggregated across the whole fleet. Fetches live from " +
			"the API (needs DOMOTZ_API_KEY).",
		Example:     "  domotz-cli fleet ip-conflicts --json",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := novelRequireLive(flags); err != nil {
				return err
			}
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			agents, err := fleetAgentsLive(cmd.Context(), c)
			if err != nil {
				return classifyAPIError(err, flags)
			}

			results, errs := cliutil.FanoutRun(cmd.Context(), agents,
				func(a fleetAgentRef) string { return a.Name },
				func(ctx context.Context, a fleetAgentRef) ([]ipConflictRow, error) {
					data, err := c.Get(ctx, fmt.Sprintf("/agent/%s/ip-conflict", url.PathEscape(a.ID)), map[string]string{})
					if err != nil {
						return nil, err
					}
					var out []ipConflictRow
					for _, item := range parseIPConflicts(data) {
						out = append(out, ipConflictRow{
							Site:               a.site(),
							AgentID:            a.ID,
							IP:                 item.IP,
							ConflictingDevices: len(item.ConflictingDevices),
						})
					}
					return out, nil
				},
				cliutil.WithConcurrency(6),
			)
			cliutil.FanoutReportErrors(cmd.ErrOrStderr(), errs)

			rows := make([]ipConflictRow, 0)
			for _, r := range results {
				rows = append(rows, r.Value...)
			}
			return printJSONFiltered(cmd.OutOrStdout(), rows, flags)
		},
	}
	return cmd
}
