// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source local

package cli

import (
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

type summaryRow struct {
	SaasCustomerID   string `json:"saas_customer_id"`
	SaasCustomerName string `json:"saas_customer_name"`
	SeatType         string `json:"seat_type"`
	Active           int    `json:"active"`
	Paused           int    `json:"paused"`
	Archived         int    `json:"archived"`
	Unprotected      int    `json:"unprotected"`
	OtherState       int    `json:"other_state"`
	Total            int    `json:"total"`
	Billable         int    `json:"billable"`
	BillableUnknown  int    `json:"billable_unknown"`
}

func newNovelSeatsSummaryCmd(flags *rootFlags) *cobra.Command {
	var dbPath, customer string
	cmd := &cobra.Command{
		Use:   "summary",
		Short: "Seat counts per client and seat type, split by state and billable flag",
		Long: strings.Trim(`
Seat counts per client and seat type: Active, Paused, Archived, Unprotected,
total, and billable. Reads the local store only; run 'datto-saas-cli sync'
first.

Use this command for plain counts by type and state.
Do NOT use it to compare counts with Datto's seatsUsed; use 'seats reconcile' instead.`, "\n"),
		Example: strings.Trim(`
  datto-saas-cli seats summary --agent
  datto-saas-cli seats summary --customer contoso.com`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "seats summary")
			}
			rows := make([]summaryRow, 0)
			db, missing, err := openSaasStore(cmd, dbPath)
			if err != nil {
				return err
			}
			if missing {
				return printSaasRows(cmd, flags, rows, "No local data. Run: datto-saas-cli sync")
			}
			defer db.Close()
			rd, err := beginSaasRead(cmd, db, flags, "domains", "seats")
			if err != nil {
				return err
			}
			defer rd.Close()
			domains, err := loadDomains(cmd.Context(), rd.tx)
			if err != nil {
				return err
			}
			customers := groupCustomers(domains)
			cid, err := customerFilter(customers, customer)
			if err != nil {
				return err
			}
			seats, err := loadSeats(cmd.Context(), rd.tx, cid)
			if err != nil {
				return err
			}
			byID := customerIndex(customers)
			idx := map[[2]string]int{}
			for _, s := range seats {
				k := [2]string{s.CustomerID, s.SeatType}
				i, ok := idx[k]
				if !ok {
					idx[k] = len(rows)
					rows = append(rows, summaryRow{SaasCustomerID: s.CustomerID, SaasCustomerName: byID[s.CustomerID].Name, SeatType: s.SeatType})
					i = len(rows) - 1
				}
				r := &rows[i]
				r.Total++
				switch s.SeatState {
				case "Active":
					r.Active++
				case "Paused":
					r.Paused++
				case "Archived":
					r.Archived++
				case "Unprotected":
					r.Unprotected++
				default:
					r.OtherState++
				}
				if b := s.billable(); b == nil {
					r.BillableUnknown++
				} else if *b {
					r.Billable++
				}
			}
			sort.SliceStable(rows, func(a, b int) bool {
				if rows[a].SaasCustomerName != rows[b].SaasCustomerName {
					return rows[a].SaasCustomerName < rows[b].SaasCustomerName
				}
				return rows[a].SeatType < rows[b].SeatType
			})
			return printSaasRows(cmd, flags, rows, emptyMsg(domains, "No seats synced."))
		},
	}
	cmd.Flags().StringVar(&customer, "customer", "", "Limit to one client: saasCustomerId, customer name, or domain")
	cmd.Flags().StringVar(&dbPath, "db", "", "SQLite database path (default: the CLI's synced store)")
	return cmd
}
