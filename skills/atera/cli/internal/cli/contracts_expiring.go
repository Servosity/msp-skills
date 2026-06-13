// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"
)

type expiringContract struct {
	ContractID   int64  `json:"ContractID"`
	ContractName string `json:"ContractName"`
	ContractType string `json:"ContractType"`
	CustomerID   int64  `json:"CustomerID"`
	CustomerName string `json:"CustomerName"`
	Active       bool   `json:"Active"`
	EndDate      string `json:"EndDate"`
	DaysToExpiry int    `json:"DaysToExpiry"` // negative = already expired (only with --include-expired)
}

// pp:data-source local
func newNovelContractsExpiringCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	var days int
	var includeInactive bool
	var includeExpired bool

	cmd := &cobra.Command{
		Use:   "expiring",
		Short: "See which contracts end inside a window, ranked by days-to-expiry and joined to the customer name.",
		Long: "Ranks synced contracts whose EndDate falls within --days from now, soonest\n" +
			"first — the renewal calendar Atera never surfaces as one view. Reads the local\n" +
			"store; run `atera-cli sync` first. Active contracts only by default;\n" +
			"--include-inactive adds the rest, --include-expired adds already-lapsed ones\n" +
			"(negative DaysToExpiry).",
		Example:     "  atera-cli contracts expiring --days 60 --agent",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			if dbPath == "" {
				dbPath = defaultDBPath("atera-cli")
			}
			s, nvOK, err := nvOpenRead(dbPath)
			if err != nil {
				return fmt.Errorf("opening store: %w", err)
			}
			if nvOK {
				defer s.Close()
			}

			if !hintIfUnsynced(cmd, s, "contracts") {
				hintIfStale(cmd, s, "contracts", flags.maxAge)
			}

			contracts, err := nvLoad(s, "contracts")
			if err != nil {
				return fmt.Errorf("loading contracts: %w", err)
			}

			now := nvNow()
			results := make([]expiringContract, 0)
			for _, c := range contracts {
				if !includeInactive && !nvBool(c, "Active") {
					continue
				}
				end, ok := nvTime(c, "EndDate")
				if !ok {
					continue // open-ended contract — nothing to rank
				}
				dte := int(end.Sub(now).Hours() / 24)
				if dte > days {
					continue
				}
				if dte < 0 && !includeExpired {
					continue
				}
				id, _ := nvInt(c, "ContractID")
				cid, _ := nvInt(c, "CustomerID")
				results = append(results, expiringContract{
					ContractID:   id,
					ContractName: nvStr(c, "ContractName"),
					ContractType: nvStr(c, "ContractType"),
					CustomerID:   cid,
					CustomerName: nvStr(c, "CustomerName"),
					Active:       nvBool(c, "Active"),
					EndDate:      nvStr(c, "EndDate"),
					DaysToExpiry: dte,
				})
			}
			// Soonest-to-expire (or most-lapsed) first.
			sort.SliceStable(results, func(i, j int) bool {
				return results[i].DaysToExpiry < results[j].DaysToExpiry
			})

			return nvEmit(cmd, flags, results, func() {
				w := cmd.OutOrStdout()
				if len(results) == 0 {
					fmt.Fprintf(w, "No contracts expiring within %d days.\n", days)
					return
				}
				rows := make([][]string, 0, len(results))
				for _, r := range results {
					eta := fmt.Sprintf("%dd", r.DaysToExpiry)
					if r.DaysToExpiry < 0 {
						eta = fmt.Sprintf("EXPIRED %dd ago", -r.DaysToExpiry)
					}
					rows = append(rows, []string{
						r.CustomerName, r.ContractName, r.ContractType, eta,
					})
				}
				nvTable(w, []string{"CUSTOMER", "CONTRACT", "TYPE", "EXPIRES"}, rows)
			})
		},
	}
	cmd.Flags().IntVar(&days, "days", 60, "Window in days — list contracts ending within this many days")
	cmd.Flags().BoolVar(&includeInactive, "include-inactive", false, "Include inactive contracts too (default: active only)")
	cmd.Flags().BoolVar(&includeExpired, "include-expired", false, "Include contracts whose EndDate has already passed")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/atera-cli/data.db)")
	return cmd
}
