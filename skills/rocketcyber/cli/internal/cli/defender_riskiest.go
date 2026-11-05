// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel feature command.
//
// pp:data-source live

package cli

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"rocketcyber-pp-cli/internal/cliutil"
)

type riskyDevice struct {
	Hostname      string `json:"hostname"`
	DeviceID      string `json:"device_id,omitempty"`
	Malicious     int64  `json:"malicious"`
	Suspicious    int64  `json:"suspicious"`
	Informational int64  `json:"informational"`
	RiskScore     int64  `json:"risk_score"`
}

type riskiestView struct {
	TotalAtRisk int64         `json:"total_at_risk"`
	Top         int           `json:"top"`
	Devices     []riskyDevice `json:"devices"`
	Note        string        `json:"note,omitempty"`
}

// rankRiskyDevices ranks Defender devices-at-risk by a weighted score
// (malicious x10 + suspicious x1), descending, capped at top.
func rankRiskyDevices(items []json.RawMessage, top int) []riskyDevice {
	devices := make([]riskyDevice, 0, len(items))
	for _, raw := range items {
		var probe map[string]json.RawMessage
		if err := json.Unmarshal(raw, &probe); err != nil {
			continue
		}
		row := riskyDevice{
			Hostname: extractString(probe, "hostname"),
			DeviceID: extractString(probe, "deviceId"),
		}
		if det, ok := probe["detections"]; ok {
			var dp map[string]json.RawMessage
			if err := json.Unmarshal(det, &dp); err == nil {
				row.Malicious, _ = cliutil.ExtractInt(dp, "malicious")
				row.Suspicious, _ = cliutil.ExtractInt(dp, "suspicious")
				row.Informational, _ = cliutil.ExtractInt(dp, "informational")
			}
		}
		row.RiskScore = row.Malicious*10 + row.Suspicious
		devices = append(devices, row)
	}
	sort.Slice(devices, func(i, j int) bool { return devices[i].RiskScore > devices[j].RiskScore })
	if top > 0 && len(devices) > top {
		devices = devices[:top]
	}
	return devices
}

// defenderDevicesAtRisk reads devicesAtRisk {total, data} from a /defender
// answer (or a top-level {total, data}). An answer without that list is an
// error, never "no devices at risk" (rocketcyber handfix
// novel-missing-input-is-an-error).
func defenderDevicesAtRisk(data json.RawMessage) ([]json.RawMessage, int64, error) {
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(data, &probe); err != nil || probe == nil {
		return nil, 0, fmt.Errorf("the answer is not a Defender report")
	}
	atRisk := probe
	if nested, ok := probe["devicesAtRisk"]; ok {
		if err := json.Unmarshal(nested, &atRisk); err != nil || atRisk == nil {
			return nil, 0, fmt.Errorf("the answer's devicesAtRisk member is not an object")
		}
	}
	var items []json.RawMessage
	if d, ok := atRisk["data"]; !ok || json.Unmarshal(d, &items) != nil || items == nil {
		return nil, 0, fmt.Errorf("the answer carries no devicesAtRisk list")
	}
	total, ok := cliutil.ExtractInt(atRisk, "total")
	if !ok {
		total = int64(len(items))
	}
	return items, total, nil
}

func newNovelDefenderRiskiestCmd(flags *rootFlags) *cobra.Command {
	var flagAccountID int
	var flagTop int

	cmd := &cobra.Command{
		Use:   "riskiest",
		Short: "Devices-at-risk ranked by weighted malicious and suspicious detection counts.",
		Long: strings.Trim(`
Ranks the Defender devices-at-risk list by a weighted risk score
(malicious x10 + suspicious x1, both from Defender detection counts),
worst first.

Use this to rank Defender devices-at-risk by detection severity. Do NOT use
it for the raw Defender health JSON; use 'defender' instead.
`, "\n"),
		Example: strings.Trim(`
  rocketcyber-cli defender riskiest --account-id 2 --top 10 --json
  rocketcyber-cli defender riskiest --top 5 --agent
`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if err := validateDataSourceStrategy(flags, "live"); err != nil {
				return err
			}
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would fetch /defender and rank devices-at-risk by weighted detection counts")
				return nil
			}
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			params := map[string]string{}
			if flagAccountID != 0 {
				params["accountId"] = strconv.Itoa(flagAccountID)
			}
			data, err := c.Get(cmd.Context(), "/defender", params)
			if err != nil {
				return classifyAPIError(cmd.OutOrStdout(), err, flags)
			}
			view := riskiestView{Top: flagTop, Devices: []riskyDevice{}}
			items, total, err := defenderDevicesAtRisk(data)
			if err != nil {
				return fmt.Errorf("/defender: %w; no ranking", err)
			}
			view.Devices = rankRiskyDevices(items, flagTop)
			view.TotalAtRisk = total
			if len(view.Devices) == 0 {
				view.Note = "no devices-at-risk returned by /defender for this scope"
			}
			return printJSONFiltered(cmd.OutOrStdout(), view, flags)
		},
	}
	cmd.Flags().IntVar(&flagAccountID, "account-id", 0, "Account ID to scope the Defender report")
	cmd.Flags().IntVar(&flagTop, "top", 10, "Number of riskiest devices to return")
	return cmd
}
