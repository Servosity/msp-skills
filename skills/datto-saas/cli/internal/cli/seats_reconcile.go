// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source local

package cli

import (
	"strings"

	"github.com/spf13/cobra"
)

type reconcileRow struct {
	SaasCustomerID     string   `json:"saas_customer_id"`
	SaasCustomerName   string   `json:"saas_customer_name"`
	Domains            []string `json:"domains"`
	SeatsUsed          *float64 `json:"seats_used"`
	Seats              int      `json:"seats"`
	Active             int      `json:"active"`
	Paused             int      `json:"paused"`
	Archived           int      `json:"archived"`
	Unprotected        int      `json:"unprotected"`
	Billable           int      `json:"billable"`
	BillableUnknown    int      `json:"billable_unknown"`
	BillableNotActive  int      `json:"billable_not_active"`
	DeltaBillableVsUse *float64 `json:"delta_billable_minus_seats_used"`
	DeltaActiveVsUse   *float64 `json:"delta_active_minus_seats_used"`
	Flags              []string `json:"flags"`
}

func newNovelSeatsReconcileCmd(flags *rootFlags) *cobra.Command {
	var dbPath, customer string
	var onlyFlagged bool
	cmd := &cobra.Command{
		Use:   "reconcile",
		Short: "See, per client, where Datto's seatsUsed disagrees with billable and Active seats before you invoice.",
		Long: strings.Trim(`
Per client, compares the seatsUsed Datto reports (summed over the client's
domains) with the billable and Active seats in the synced seat list, and
reports signed deltas plus flags:

  seats_used_mismatch          seatsUsed differs from the billable seat count
  billable_not_active          billable seats that are Paused, Archived or Unprotected
  missing_subscription         a domain has no externalSubscriptionId
  zero_seats_used_with_active  seatsUsed is 0 while Active seats exist
  no_seats_synced              domains exist but no seats were synced
  billable_unknown             some seats carry no parseable billable flag; no
                               billable delta or seats_used_mismatch is reported
  local_mirror_incomplete      the last sync of domains or seats did not finish, so
                               no deltas or mismatches are claimed
  seats_used_incomplete        not every domain reported seatsUsed; seats_used is a
                               partial sum (null when none reported) and no deltas
                               are computed

Deltas are arithmetic only; Datto does not publish how seatsUsed is counted.

Use this command to reconcile what Datto counts against what is deployed for each client: summed domain seatsUsed vs billable seats vs Active seats, with signed deltas and flags.
Do NOT use this command to list the individual unprotected seats; use 'seats gaps' instead.
Do NOT use it for plain seat counts by type and state; use 'seats summary' instead.`, "\n"),
		Example: strings.Trim(`
  datto-saas-cli seats reconcile --agent
  datto-saas-cli seats reconcile --only-flagged --csv`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "seats reconcile")
			}
			rows := make([]reconcileRow, 0)
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
			partial := rd.partial
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
			byCustomer := map[string][]saasSeat{}
			for _, s := range seats {
				byCustomer[s.CustomerID] = append(byCustomer[s.CustomerID], s)
			}
			for _, c := range customers {
				if cid != "" && c.ID != cid {
					continue
				}
				r := reconcileRow{SaasCustomerID: c.ID, SaasCustomerName: c.Name, Domains: c.domainNames(), Flags: make([]string, 0)}
				for _, s := range byCustomer[c.ID] {
					r.Seats++
					switch s.SeatState {
					case "Active":
						r.Active++
					case "Paused":
						r.Paused++
					case "Archived":
						r.Archived++
					case "Unprotected":
						r.Unprotected++
					}
					b := s.billable()
					switch {
					case b == nil:
						r.BillableUnknown++
					case *b:
						r.Billable++
						if s.SeatState != "Active" {
							r.BillableNotActive++
						}
					}
				}
				if !c.seatsUsedComplete() {
					if c.SeatsUsedSeen {
						used := c.SeatsUsed
						r.SeatsUsed = &used
					}
					r.Flags = append(r.Flags, "seats_used_incomplete")
				}
				if len(partial) > 0 {
					// An unfinished sync can hide domains or seats: report counts but
					// claim no deltas or mismatches.
					r.Flags = append(r.Flags, "local_mirror_incomplete")
				}
				if c.seatsUsedComplete() && len(partial) == 0 {
					used := c.SeatsUsed
					r.SeatsUsed = &used
					da := float64(r.Active) - used
					r.DeltaActiveVsUse = &da
					// Unknown billable flags make the billable count a lower bound,
					// so no billable delta or mismatch is claimed.
					if r.BillableUnknown == 0 {
						db := float64(r.Billable) - used
						r.DeltaBillableVsUse = &db
						if db != 0 {
							r.Flags = append(r.Flags, "seats_used_mismatch")
						}
					}
					if used == 0 && r.Active > 0 {
						r.Flags = append(r.Flags, "zero_seats_used_with_active")
					}
				}
				if r.BillableNotActive > 0 {
					r.Flags = append(r.Flags, "billable_not_active")
				}
				for _, d := range c.Domains {
					if strings.TrimSpace(d.SubscriptionID) == "" {
						r.Flags = append(r.Flags, "missing_subscription")
						break
					}
				}
				if r.Seats == 0 {
					r.Flags = append(r.Flags, "no_seats_synced")
				}
				if r.BillableUnknown > 0 {
					r.Flags = append(r.Flags, "billable_unknown")
				}
				if onlyFlagged && len(r.Flags) == 0 {
					continue
				}
				rows = append(rows, r)
			}
			return printSaasRows(cmd, flags, rows, emptyMsg(domains, "No clients to reconcile."))
		},
	}
	cmd.Flags().StringVar(&customer, "customer", "", "Limit to one client: saasCustomerId, customer name, or domain")
	cmd.Flags().BoolVar(&onlyFlagged, "only-flagged", false, "Only show clients with at least one flag")
	cmd.Flags().StringVar(&dbPath, "db", "", "SQLite database path (default: the CLI's synced store)")
	return cmd
}
