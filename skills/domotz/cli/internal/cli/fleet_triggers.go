// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel feature. Fleet down-sensor sweep: TCP service sensors
// (eyes) currently DOWN across every device on every agent. The API exposes
// eyes per agent only; the fleet-wide view is a live fan-out. SNMP eyes carry
// no live status at the per-agent list level (their trigger state is a
// per-sensor endpoint), so this command reports TCP service checks — the
// sensor type with a fleet-sweepable UP/DOWN status.

package cli

import (
	"context"
	"domotz-pp-cli/internal/cliutil"
	"encoding/json"
	"fmt"
	"net/url"

	"github.com/spf13/cobra"
)

// tcpEyeItem mirrors TCPDomotzEye: a TCP service check bound to a device.
type tcpEyeItem struct {
	ID         json.RawMessage `json:"id"`
	DeviceID   json.RawMessage `json:"device_id"`
	Port       int             `json:"port"`
	Status     string          `json:"status"`
	LastUpdate string          `json:"last_update"`
}

// fleetTriggerRow is one DOWN TCP sensor labeled with its site.
type fleetTriggerRow struct {
	Site       string `json:"site"`
	AgentID    string `json:"agent_id"`
	DeviceID   string `json:"device_id"`
	EyeID      string `json:"eye_id"`
	Port       int    `json:"port"`
	Status     string `json:"status"`
	LastUpdate string `json:"last_update"`
}

// pp:data-source live
func newNovelFleetTriggersCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "triggers",
		Short: "TCP service sensors (eyes) currently DOWN across the whole fleet",
		Long: "Sweep every agent's TCP service sensors (eyes) and report the ones currently DOWN, " +
			"labeled with their site — one fleet-wide list the per-agent eye endpoints can't give you. " +
			"Use this command for currently-down TCP service checks across the fleet. " +
			"Do NOT use this command for alert-profile rules/bindings; use 'fleet alerts' instead. " +
			"SNMP sensor trigger state is per-sensor in the API and is not swept here. " +
			"Fetches live from the API (needs DOMOTZ_API_KEY).",
		Example:     "  domotz-cli fleet triggers --json --select site,port,last_update",
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
				func(ctx context.Context, a fleetAgentRef) ([]fleetTriggerRow, error) {
					data, err := c.Get(ctx, fmt.Sprintf("/agent/%s/device/eye/tcp", url.PathEscape(a.ID)), map[string]string{})
					if err != nil {
						return nil, err
					}
					var eyes []tcpEyeItem
					if err := json.Unmarshal(data, &eyes); err != nil {
						return nil, fmt.Errorf("parsing TCP eyes for agent %s: %w", a.ID, err)
					}
					out := make([]fleetTriggerRow, 0)
					for _, eye := range eyes {
						if eye.Status != "DOWN" {
							continue
						}
						out = append(out, fleetTriggerRow{
							Site:       a.site(),
							AgentID:    a.ID,
							DeviceID:   trimJSONString(eye.DeviceID),
							EyeID:      trimJSONString(eye.ID),
							Port:       eye.Port,
							Status:     eye.Status,
							LastUpdate: eye.LastUpdate,
						})
					}
					return out, nil
				},
				cliutil.WithConcurrency(6),
			)
			cliutil.FanoutReportErrors(cmd.ErrOrStderr(), errs)

			rows := make([]fleetTriggerRow, 0)
			for _, r := range results {
				rows = append(rows, r.Value...)
			}
			return printJSONFiltered(cmd.OutOrStdout(), rows, flags)
		},
	}
	return cmd
}
