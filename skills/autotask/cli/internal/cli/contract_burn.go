// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
package cli

import (
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

// newNovelContractBurnCmd rolls up contract block hours purchased vs consumed
// per contract — a local aggregation across every synced contract block.
// pp:data-source local
func newNovelContractBurnCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	cmd := &cobra.Command{
		Use:   "contract-burn",
		Short: "Show how much of each contract's hours/blocks are consumed versus purchased.",
		Long: `Roll up contract block hours purchased vs consumed per contract, flagging contracts that are nearly exhausted. Run ` + "`sync`" + ` first.

Use this command for the consumed-vs-purchased snapshot. For run-out projection and over-threshold flags, use 'retainer'.`,
		Example: strings.Trim(`
  autotask-cli contract-burn
  autotask-cli contract-burn --agent
  autotask-cli contract-burn --json`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			db, err := openNovelStore(cmd.Context(), dbPath)
			if err != nil {
				return err
			}
			defer db.Close()
			if !hintIfUnsynced(cmd, db, "contract-blocks") {
				hintIfStale(cmd, db, "contract-blocks", flags.maxAge)
			}
			blocks, err := listEntity(db, "contract-blocks")
			if err != nil {
				return apiErr(err)
			}
			type burn struct {
				ContractID     string  `json:"contractID"`
				HoursPurchased float64 `json:"hoursPurchased"`
				HoursUsed      float64 `json:"hoursUsed"`
				HoursRemaining float64 `json:"hoursRemaining"`
				PercentBurned  float64 `json:"percentBurned"`
			}
			byContract := map[string]*burn{}
			for _, b := range blocks {
				cid := strAt(b, "contractID", "contractId")
				if cid == "" {
					cid = "(unknown)"
				}
				if byContract[cid] == nil {
					byContract[cid] = &burn{ContractID: cid}
				}
				bc := byContract[cid]
				purchased, used, remaining := accrueBlock(b)
				bc.HoursPurchased += purchased
				bc.HoursUsed += used
				bc.HoursRemaining += remaining
			}
			rows := make([]burn, 0, len(byContract))
			for _, bc := range byContract {
				if bc.HoursPurchased > 0 {
					bc.PercentBurned = (bc.HoursUsed / bc.HoursPurchased) * 100
				}
				rows = append(rows, *bc)
			}
			sort.Slice(rows, func(i, j int) bool { return rows[i].PercentBurned > rows[j].PercentBurned })
			return flags.printJSON(cmd, rows)
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/autotask-cli/data.db)")
	return cmd
}
