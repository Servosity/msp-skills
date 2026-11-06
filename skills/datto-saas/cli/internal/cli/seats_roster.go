// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source local

package cli

import (
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

type rosterRow struct {
	SaasCustomerID   string   `json:"saas_customer_id"`
	SaasCustomerName string   `json:"saas_customer_name"`
	OrganizationName string   `json:"organization_name,omitempty"`
	Domains          []string `json:"domains"`
	ProductTypes     []string `json:"product_types"`
	Subscriptions    []string `json:"external_subscription_ids"`
	Name             string   `json:"name"`
	MainID           string   `json:"main_id"`
	SeatType         string   `json:"seat_type"`
	SeatState        string   `json:"seat_state"`
	Billable         *bool    `json:"billable"`
	DateAdded        string   `json:"date_added"`
	RemoteID         string   `json:"remote_id"`
}

func newNovelSeatsRosterCmd(flags *rootFlags) *cobra.Command {
	var dbPath, customer, states, seatTypes string
	cmd := &cobra.Command{
		Use:   "roster",
		Short: "Every synced seat across all clients, joined with its customer's domains, product and subscription ids",
		Long: strings.Trim(`
Every synced seat across all clients in one table, joined with the customer's
domains, product types, organization and external subscription ids. Reads the
local store only; run 'datto-saas-cli sync' first.

Use this command for a fleet-wide seat export (CSV with --csv).
Do NOT use it for counts by state; use 'seats summary' instead.`, "\n"),
		Example: strings.Trim(`
  datto-saas-cli seats roster --csv
  datto-saas-cli seats roster --customer 123456 --state Unprotected,Paused --agent`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "seats roster")
			}
			stateSet, typeSet := splitCSVSet(states), splitCSVSet(seatTypes)
			if err := validateEnumSet("state", stateSet, saasSeatStates); err != nil {
				return err
			}
			if err := validateEnumSet("seat-type", typeSet, saasSeatTypes); err != nil {
				return err
			}
			rows := make([]rosterRow, 0)
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
			for _, s := range seats {
				if len(stateSet) > 0 && !stateSet[s.SeatState] {
					continue
				}
				if len(typeSet) > 0 && !typeSet[s.SeatType] {
					continue
				}
				c := byID[s.CustomerID]
				rows = append(rows, rosterRow{
					SaasCustomerID: s.CustomerID, SaasCustomerName: c.Name, OrganizationName: c.OrgName,
					Domains: c.domainNames(), ProductTypes: c.productTypes(), Subscriptions: c.subscriptions(),
					Name: s.Name, MainID: s.MainID, SeatType: s.SeatType, SeatState: s.SeatState,
					Billable: s.billable(), DateAdded: string(s.DateAdded), RemoteID: s.RemoteID,
				})
			}
			sort.SliceStable(rows, func(a, b int) bool {
				if rows[a].SaasCustomerName != rows[b].SaasCustomerName {
					return rows[a].SaasCustomerName < rows[b].SaasCustomerName
				}
				return rows[a].MainID < rows[b].MainID
			})
			return printSaasRows(cmd, flags, rows, emptyMsg(domains, "No seats match."))
		},
	}
	cmd.Flags().StringVar(&customer, "customer", "", "Limit to one client: saasCustomerId, customer name, or domain")
	cmd.Flags().StringVar(&states, "state", "", "Comma-separated seat states to keep: Active, Paused, Archived, Unprotected")
	cmd.Flags().StringVar(&seatTypes, "seat-type", "", "Comma-separated seat types to keep: User, SharedMailbox, SharedDrive, Site, TeamSite, Team")
	cmd.Flags().StringVar(&dbPath, "db", "", "SQLite database path (default: the CLI's synced store)")
	return cmd
}
