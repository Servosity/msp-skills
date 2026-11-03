// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel feature (added in the 4.34.2 reprint). Alert-coverage
// audit: the devices no enabled alert profile watches, across every Collector,
// VITAL first: the devices that can go down without anyone being told. The API
// answers it per Collector in two calls (its device list and its device alert
// profile bindings) plus the profile list; no endpoint answers it fleet-wide.
// Live fan-out across the agent list.

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"

	"domotz-pp-cli/internal/cliutil"

	"github.com/spf13/cobra"
)

type unalertedRow struct {
	Site        string `json:"site"`
	AgentID     string `json:"agent_id"`
	DeviceID    string `json:"device_id"`
	DisplayName string `json:"display_name"`
	Importance  string `json:"importance"`
	Status      string `json:"status"`
	Type        string `json:"type"`
}

// alertProfileItem, alertProfileDeviceBinding and unalertedDeviceItem are
// the fields read from GET /alert-profile (AlertProfile), GET
// /alert-profile/binding/agent/{agent_id}/device (AlertProfileDeviceBinding)
// and GET /agent/{agent_id}/device (LocalIpDevice and its siblings); pinned
// against spec.json by TestFleetFieldsAreDocumented.
type alertProfileItem struct {
	ID        json.RawMessage `json:"id"`
	IsEnabled bool            `json:"is_enabled"`
}

type alertProfileDeviceBinding struct {
	AlertProfileID json.RawMessage `json:"alert_profile_id"`
	DeviceID       json.RawMessage `json:"device_id"`
}

type unalertedDeviceItem struct {
	ID          json.RawMessage `json:"id"`
	DisplayName string          `json:"display_name"`
	Importance  string          `json:"importance"`
	Status      string          `json:"status"`
	Type        struct {
		Label string `json:"label"`
	} `json:"type"`
}

// enabledAlertProfiles is the set of alert profile ids that are enabled
// (GET /alert-profile, AlertProfile id and is_enabled). A binding to a profile
// missing from the list or disabled watches nothing.
func enabledAlertProfiles(ctx context.Context, get func(context.Context, string, map[string]string) (json.RawMessage, error)) (map[string]bool, error) {
	data, err := get(ctx, "/alert-profile", map[string]string{})
	if err != nil {
		return nil, err
	}
	var profiles []alertProfileItem
	if err := json.Unmarshal(data, &profiles); err != nil {
		return nil, fmt.Errorf("parsing alert profiles: %w", err)
	}
	enabled := map[string]bool{}
	for _, p := range profiles {
		if p.IsEnabled {
			enabled[trimJSONString(p.ID)] = true
		}
	}
	return enabled, nil
}

// unalertedDevices reads one Collector's devices and its device alert profile
// bindings (AlertProfileDeviceBinding: alert_profile_id, device_id) and
// returns the devices no enabled profile is bound to.
func unalertedDevices(ctx context.Context, get func(context.Context, string, map[string]string) (json.RawMessage, error), a fleetAgentRef, enabled map[string]bool) ([]unalertedRow, error) {
	id := url.PathEscape(a.ID)
	data, err := get(ctx, "/alert-profile/binding/agent/"+id+"/device", map[string]string{})
	if err != nil {
		return nil, err
	}
	var bindings []alertProfileDeviceBinding
	if err := json.Unmarshal(data, &bindings); err != nil {
		return nil, fmt.Errorf("parsing alert profile bindings for agent %s: %w", a.ID, err)
	}
	watched := map[string]bool{}
	for _, b := range bindings {
		if enabled[trimJSONString(b.AlertProfileID)] {
			watched[trimJSONString(b.DeviceID)] = true
		}
	}
	data, err = get(ctx, "/agent/"+id+"/device", map[string]string{})
	if err != nil {
		return nil, err
	}
	var devices []unalertedDeviceItem
	if err := json.Unmarshal(data, &devices); err != nil {
		return nil, fmt.Errorf("parsing devices for agent %s: %w", a.ID, err)
	}
	out := make([]unalertedRow, 0)
	for _, d := range devices {
		did := trimJSONString(d.ID)
		if did == "" || watched[did] {
			continue
		}
		out = append(out, unalertedRow{Site: a.site(), AgentID: a.ID, DeviceID: did, DisplayName: d.DisplayName,
			Importance: d.Importance, Status: d.Status, Type: d.Type.Label})
	}
	return out, nil
}

// pp:data-source live
func newNovelFleetUnalertedCmd(flags *rootFlags) *cobra.Command {
	var vitalOnly bool
	cmd := &cobra.Command{
		Use:   "unalerted",
		Short: "Devices no enabled alert profile watches, across every site, VITAL first",
		Long: "Audit alert coverage across every Domotz Collector: list the devices that no enabled alert " +
			"profile is bound to, so they can go down without anyone being told. VITAL devices sort first. " +
			"Profiles bound to a Collector (not to a device) are not counted as watching its devices. " +
			"Fetches live from the API (needs DOMOTZ_API_KEY): the profile list, then each Collector's " +
			"devices and device bindings.",
		Example:     "  domotz-cli fleet unalerted --vital-only --json",
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
			enabled, err := enabledAlertProfiles(cmd.Context(), c.Get)
			if err != nil {
				return classifyAPIError(cmd.OutOrStdout(), err, flags)
			}
			agents, err := fleetAgentsLive(cmd.Context(), c)
			if err != nil {
				return classifyAPIError(cmd.OutOrStdout(), err, flags)
			}
			results, errs := cliutil.FanoutRun(cmd.Context(), agents,
				func(a fleetAgentRef) string { return a.Name },
				func(ctx context.Context, a fleetAgentRef) ([]unalertedRow, error) {
					return unalertedDevices(ctx, c.Get, a, enabled)
				},
				cliutil.WithConcurrency(6),
			)
			cliutil.FanoutReportErrors(cmd.ErrOrStderr(), errs)

			rows := make([]unalertedRow, 0)
			for _, r := range results {
				for _, row := range r.Value {
					if vitalOnly && row.Importance != "VITAL" {
						continue
					}
					rows = append(rows, row)
				}
			}
			sort.SliceStable(rows, func(i, j int) bool {
				if (rows[i].Importance == "VITAL") != (rows[j].Importance == "VITAL") {
					return rows[i].Importance == "VITAL"
				}
				if rows[i].Site != rows[j].Site {
					return rows[i].Site < rows[j].Site
				}
				return rows[i].DisplayName < rows[j].DisplayName
			})
			if err := printJSONFiltered(cmd.OutOrStdout(), rows, flags); err != nil {
				return err
			}
			return fanoutIncomplete(errs, len(agents))
		},
	}
	cmd.Flags().BoolVar(&vitalOnly, "vital-only", false, "List only VITAL devices")
	return cmd
}
