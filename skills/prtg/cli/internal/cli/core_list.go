// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: core list shows registered cores and whether PRTG_FLEET_KEYS carries each one's key.
// pp:data-source local

package cli

import (
	"encoding/json"

	"github.com/spf13/cobra"

	"prtg-pp-cli/internal/fleet"
)

func newCoreListCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List registered PRTG cores and whether PRTG_FLEET_KEYS carries each one's API key",
		Example:     "  prtg-cli core list --json",
		Annotations: map[string]string{"pp:data-source": "local", "mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "core list")
			}
			reg, err := fleet.Load()
			if err != nil {
				return configErr(err)
			}
			keys := fleetKeysOrWarn(cmd)
			rows := make([]coreListRow, 0, len(reg.Cores))
			for _, c := range reg.Cores {
				rows = append(rows, coreRow(c, keys))
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), rows, flags)
			}
			if len(rows) == 0 {
				hintEmptyFleet(cmd)
				return nil
			}
			data, _ := json.Marshal(rows)
			var items []map[string]any
			_ = json.Unmarshal(data, &items)
			return printAutoTable(cmd.OutOrStdout(), items)
		},
	}
	return cmd
}
