// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"
)

type bookContract struct {
	ContractName string `json:"ContractName"`
	ContractType string `json:"ContractType"`
	Active       bool   `json:"Active"`
}

type bookEntry struct {
	CustomerID          int64          `json:"CustomerID"`
	CustomerName        string         `json:"CustomerName"`
	AgentCount          int            `json:"AgentCount"`
	ContractCount       int            `json:"ContractCount"`
	ActiveContractCount int            `json:"ActiveContractCount"`
	Contracts           []bookContract `json:"Contracts"`
}

// pp:data-source local
func newNovelCustomersBookCmd(flags *rootFlags) *cobra.Command {
	var dbPath string

	cmd := &cobra.Command{
		Use:   "book",
		Short: "Per-customer rollup: agent count and contract mix, joining customers, contracts, and agents.",
		Long: "Use this command for a full per-customer footprint + contract-mix rollup. Do NOT use it to isolate under-contracted accounts; use 'customers coverage' instead.\n\n" +
			"Builds a book-of-business view by joining synced customers with their contracts\n" +
			"and agent counts. Reads the local store; run `atera-cli sync` first. The join\n" +
			"across customers, contracts, and agents only exists once everything is local.",
		Example:     "  atera-cli customers book --agent",
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

			if !hintIfUnsynced(cmd, s, "customers") {
				hintIfStale(cmd, s, "customers", flags.maxAge)
			}

			customers, err := nvLoad(s, "customers")
			if err != nil {
				return fmt.Errorf("loading customers: %w", err)
			}
			contracts, err := nvLoad(s, "contracts")
			if err != nil {
				return fmt.Errorf("loading contracts: %w", err)
			}
			agents, err := nvLoad(s, "agents")
			if err != nil {
				return fmt.Errorf("loading agents: %w", err)
			}

			// Index agents and contracts by CustomerID.
			agentCount := map[int64]int{}
			for _, a := range agents {
				if cid, ok := nvInt(a, "CustomerID"); ok {
					agentCount[cid]++
				}
			}
			contractsByCust := map[int64][]bookContract{}
			activeByCust := map[int64]int{}
			for _, c := range contracts {
				cid, ok := nvInt(c, "CustomerID")
				if !ok {
					continue
				}
				active := nvBool(c, "Active")
				contractsByCust[cid] = append(contractsByCust[cid], bookContract{
					ContractName: nvStr(c, "ContractName"),
					ContractType: nvStr(c, "ContractType"),
					Active:       active,
				})
				if active {
					activeByCust[cid]++
				}
			}

			results := make([]bookEntry, 0, len(customers))
			for _, cust := range customers {
				cid, _ := nvInt(cust, "CustomerID")
				cs := contractsByCust[cid]
				if cs == nil {
					cs = []bookContract{}
				}
				results = append(results, bookEntry{
					CustomerID:          cid,
					CustomerName:        nvStr(cust, "CustomerName"),
					AgentCount:          agentCount[cid],
					ContractCount:       len(cs),
					ActiveContractCount: activeByCust[cid],
					Contracts:           cs,
				})
			}
			// Biggest estates first.
			sort.SliceStable(results, func(i, j int) bool {
				if results[i].AgentCount != results[j].AgentCount {
					return results[i].AgentCount > results[j].AgentCount
				}
				return results[i].ActiveContractCount > results[j].ActiveContractCount
			})

			return nvEmit(cmd, flags, results, func() {
				w := cmd.OutOrStdout()
				if len(results) == 0 {
					fmt.Fprintln(w, "No customers in the local store. Run `atera-cli sync` first.")
					return
				}
				rows := make([][]string, 0, len(results))
				for _, r := range results {
					rows = append(rows, []string{
						r.CustomerName,
						fmt.Sprintf("%d", r.AgentCount),
						fmt.Sprintf("%d", r.ContractCount),
						fmt.Sprintf("%d", r.ActiveContractCount),
					})
				}
				nvTable(w, []string{"CUSTOMER", "AGENTS", "CONTRACTS", "ACTIVE"}, rows)
			})
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/atera-cli/data.db)")
	return cmd
}
