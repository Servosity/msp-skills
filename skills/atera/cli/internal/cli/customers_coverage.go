// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

type coverageEntry struct {
	CustomerID         int64    `json:"CustomerID"`
	CustomerName       string   `json:"CustomerName"`
	AgentCount         int      `json:"AgentCount"`
	ActiveContracts    int      `json:"ActiveContracts"`
	RecurringContracts int      `json:"RecurringContracts"`
	Covered            bool     `json:"Covered"`
	ActiveTypes        []string `json:"ActiveTypes"`
}

// isRecurringContractType reports whether an active contract of this type
// counts as recurring coverage for a managed estate. Atera's polymorphic
// contract types include "Remote Monitoring", "Retainer Flat Fee", "Block
// Hours", "Block Money", "Hourly", "Online Backup", and the Project* one-time
// shapes; only monitoring/retainer types represent ongoing managed coverage.
func isRecurringContractType(t string) bool {
	n := strings.ReplaceAll(strings.ToLower(strings.TrimSpace(t)), " ", "")
	return strings.Contains(n, "monitoring") || strings.Contains(n, "retainer")
}

// pp:data-source local
func newNovelCustomersCoverageCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	var showAll bool
	var anyActive bool

	cmd := &cobra.Command{
		Use:   "coverage",
		Short: "Find under-contracted customers — managed agents with no active recurring or monitoring contract.",
		Long: "Use this command to find under-contracted customers — managed agents with no\n" +
			"active recurring/monitoring contract. Do NOT use it for a full footprint +\n" +
			"contract-mix rollup; use 'customers book' instead.\n\n" +
			"Joins synced agents and contracts per customer, then filters to customers that\n" +
			"have managed endpoints but no active Remote Monitoring / Retainer contract —\n" +
			"the margin leak no Atera screen surfaces. Reads the local store; run\n" +
			"`atera-cli sync` first. Use --any-active to treat any active contract as\n" +
			"coverage, or --all to list covered customers too.",
		Example:     "  atera-cli customers coverage --agent",
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

			agentCount := map[int64]int{}
			for _, a := range agents {
				if cid, ok := nvInt(a, "CustomerID"); ok {
					agentCount[cid]++
				}
			}
			activeByCust := map[int64]int{}
			recurringByCust := map[int64]int{}
			typesByCust := map[int64][]string{}
			for _, c := range contracts {
				cid, ok := nvInt(c, "CustomerID")
				if !ok || !nvBool(c, "Active") {
					continue
				}
				ct := nvStr(c, "ContractType")
				activeByCust[cid]++
				typesByCust[cid] = append(typesByCust[cid], ct)
				if isRecurringContractType(ct) {
					recurringByCust[cid]++
				}
			}

			results := make([]coverageEntry, 0, len(customers))
			for _, cust := range customers {
				cid, _ := nvInt(cust, "CustomerID")
				covered := recurringByCust[cid] > 0
				if anyActive {
					covered = activeByCust[cid] > 0
				}
				e := coverageEntry{
					CustomerID:         cid,
					CustomerName:       nvStr(cust, "CustomerName"),
					AgentCount:         agentCount[cid],
					ActiveContracts:    activeByCust[cid],
					RecurringContracts: recurringByCust[cid],
					Covered:            covered,
					ActiveTypes:        typesByCust[cid],
				}
				if e.ActiveTypes == nil {
					e.ActiveTypes = []string{}
				}
				// The gap view: managed endpoints without coverage. --all keeps everything.
				if !showAll && (e.Covered || e.AgentCount == 0) {
					continue
				}
				results = append(results, e)
			}
			// Biggest uncovered estates first; covered rows (only with --all) sink.
			sort.SliceStable(results, func(i, j int) bool {
				if results[i].Covered != results[j].Covered {
					return !results[i].Covered
				}
				return results[i].AgentCount > results[j].AgentCount
			})

			return nvEmit(cmd, flags, results, func() {
				w := cmd.OutOrStdout()
				if len(results) == 0 {
					fmt.Fprintln(w, "No under-contracted customers — every managed estate has active recurring coverage.")
					return
				}
				rows := make([][]string, 0, len(results))
				for _, r := range results {
					rows = append(rows, []string{
						r.CustomerName,
						fmt.Sprintf("%d", r.AgentCount),
						fmt.Sprintf("%d", r.ActiveContracts),
						fmt.Sprintf("%d", r.RecurringContracts),
						fmt.Sprintf("%t", r.Covered),
					})
				}
				nvTable(w, []string{"CUSTOMER", "AGENTS", "ACTIVE", "RECURRING", "COVERED"}, rows)
			})
		},
	}
	cmd.Flags().BoolVar(&showAll, "all", false, "Include covered customers and customers with zero agents")
	cmd.Flags().BoolVar(&anyActive, "any-active", false, "Treat any active contract as coverage (default: only monitoring/retainer types)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/atera-cli/data.db)")
	return cmd
}
