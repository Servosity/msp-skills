// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
package cli

import (
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

// newNovelUnbilledCmd lists billable time not yet approved and posted for
// billing — hours that never reach an invoice are money on the table.
// pp:data-source local
func newNovelUnbilledCmd(flags *rootFlags) *cobra.Command {
	var flagCompany string
	var dbPath string
	cmd := &cobra.Command{
		Use:   "unbilled",
		Short: "Surface billable time entries not yet approved and posted for billing — money on the table.",
		Long: `List billable time entries not yet approved and posted for billing (no billingApprovalDateTime), with the hours each would bill, optionally filtered to one company (through each entry's ticket, task project or contract). Run ` + "`sync`" + ` first.

Use this command for the fast unbilled-hours answer. For the full month-end reconciliation across time, invoices, and contracts, use 'reconcile'.`,
		Example: strings.Trim(`
  autotask-cli unbilled
  autotask-cli unbilled --company 1234 --agent
  autotask-cli unbilled --json`, "\n"),
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
			entries, err := listEntity(db, "time-entries")
			if err != nil {
				return apiErr(err)
			}
			type entry struct {
				ID         int64   `json:"id"`
				CompanyID  string  `json:"companyID,omitempty"`
				TicketID   string  `json:"ticketID,omitempty"`
				ResourceID string  `json:"resourceID,omitempty"`
				Hours      float64 `json:"hours"`
				DateWorked string  `json:"dateWorked,omitempty"`
			}
			companies, err := loadTimeEntryCompanies(db)
			if err != nil {
				return apiErr(err)
			}
			var rows []entry
			var totalHours float64
			for _, e := range entries {
				if !timeEntryUnbilled(e) {
					continue
				}
				cid := companies.company(e)
				if flagCompany != "" && cid != flagCompany {
					continue
				}
				id, _ := intAt(e, "id")
				h := timeEntryBillHours(e)
				totalHours += h
				rows = append(rows, entry{
					ID:         id,
					CompanyID:  cid,
					TicketID:   strAt(e, "ticketID", "ticketId"),
					ResourceID: strAt(e, "resourceID", "resourceId"),
					Hours:      h,
					DateWorked: strAt(e, "dateWorked", "startDateTime"),
				})
			}
			sort.Slice(rows, func(i, j int) bool { return rows[i].Hours > rows[j].Hours })
			out := map[string]any{
				"unbilledEntries": rows,
				"totalEntries":    len(rows),
				"totalHours":      totalHours,
			}
			return flags.printJSON(cmd, out)
		},
	}
	cmd.Flags().StringVar(&flagCompany, "company", "", "limit to a single company id")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/autotask-cli/data.db)")
	return cmd
}
