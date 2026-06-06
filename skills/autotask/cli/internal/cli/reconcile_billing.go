// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
package cli

import (
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

// newNovelReconcileCmd is the month-end billing close in one table: approved
// time vs invoiced, contract blocks consumed vs purchased, and the
// money-on-the-table total. LEFT-JOINs TimeEntries x Invoices x Contracts
// locally — Autotask only shows these halves on separate screens.
// pp:data-source local
func newNovelReconcileCmd(flags *rootFlags) *cobra.Command {
	var flagCompany string
	var dbPath string
	cmd := &cobra.Command{
		Use:   "reconcile",
		Short: "One month-end table: unbilled approved time, contract burn, and the money left on the table.",
		Long: `Cross-join time entries, invoices, and contract blocks in the local store into one billing-close report: approved-but-uninvoiced hours (by company), contract consumed-vs-purchased, and a money-on-the-table total. Run ` + "`sync`" + ` first.

Use this command for the full month-end reconciliation across time, invoices, and contracts. Do NOT use it for just unbilled hours (use 'unbilled') or just per-contract consumption (use 'contract-burn').`,
		Example: strings.Trim(`
  autotask-cli reconcile
  autotask-cli reconcile --company 1234 --agent
  autotask-cli reconcile --json --select unbilled,moneyOnTheTableHours`, "\n"),
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
			if !hintIfUnsynced(cmd, db, "time-entries") {
				hintIfStale(cmd, db, "time-entries", flags.maxAge)
			}
			if !hintIfUnsynced(cmd, db, "contract-blocks") {
				hintIfStale(cmd, db, "contract-blocks", flags.maxAge)
			}

			entries, err := listEntity(db, "time-entries")
			if err != nil {
				return apiErr(err)
			}
			blocks, _ := listEntity(db, "contract-blocks")
			contracts, _ := listEntity(db, "contracts")

			// Map contractID -> companyID so contract rows can honor --company.
			contractCompany := map[string]string{}
			for _, c := range contracts {
				if cid := strAt(c, "id"); cid != "" {
					contractCompany[cid] = strAt(c, "companyID", "companyId")
				}
			}

			// Leg 1: unbilled approved time (no invoiceID, billable).
			type companyHours struct {
				CompanyID string  `json:"companyID"`
				Entries   int     `json:"entries"`
				Hours     float64 `json:"hours"`
			}
			byCompany := map[string]*companyHours{}
			var unbilledEntries int
			var unbilledHours float64
			for _, e := range entries {
				if inv, ok := intAt(e, "invoiceID", "invoiceId"); ok && inv != 0 {
					continue
				}
				if boolAt(e, "isNonBillable") {
					continue
				}
				cid := strAt(e, "companyID", "companyId")
				if flagCompany != "" && cid != flagCompany {
					continue
				}
				h, _ := numAt(e, "hoursWorked", "hoursToBill", "hours")
				unbilledEntries++
				unbilledHours += h
				key := cid
				if key == "" {
					key = "(unknown)"
				}
				if byCompany[key] == nil {
					byCompany[key] = &companyHours{CompanyID: key}
				}
				byCompany[key].Entries++
				byCompany[key].Hours += h
			}
			companyRows := make([]companyHours, 0, len(byCompany))
			for _, ch := range byCompany {
				companyRows = append(companyRows, *ch)
			}
			sort.Slice(companyRows, func(i, j int) bool { return companyRows[i].Hours > companyRows[j].Hours })

			// Leg 2: contract blocks consumed vs purchased.
			type burn struct {
				ContractID     string  `json:"contractID"`
				CompanyID      string  `json:"companyID,omitempty"`
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
				if flagCompany != "" && contractCompany[cid] != flagCompany {
					continue
				}
				if byContract[cid] == nil {
					byContract[cid] = &burn{ContractID: cid, CompanyID: contractCompany[cid]}
				}
				bc := byContract[cid]
				purchased, used, remaining := accrueBlock(b)
				bc.HoursPurchased += purchased
				bc.HoursUsed += used
				bc.HoursRemaining += remaining
			}
			contractRows := make([]burn, 0, len(byContract))
			for _, bc := range byContract {
				if bc.HoursPurchased > 0 {
					bc.PercentBurned = (bc.HoursUsed / bc.HoursPurchased) * 100
				}
				contractRows = append(contractRows, *bc)
			}
			sort.Slice(contractRows, func(i, j int) bool { return contractRows[i].PercentBurned > contractRows[j].PercentBurned })

			out := map[string]any{
				"company": flagCompany,
				"unbilled": map[string]any{
					"totalEntries": unbilledEntries,
					"totalHours":   unbilledHours,
					"byCompany":    companyRows,
				},
				"contracts":            contractRows,
				"moneyOnTheTableHours": unbilledHours,
			}
			return flags.printJSON(cmd, out)
		},
	}
	cmd.Flags().StringVar(&flagCompany, "company", "", "limit the reconciliation to a single company id")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/autotask-cli/data.db)")
	return cmd
}
