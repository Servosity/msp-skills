// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source local

package cli

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"datto-saas-pp-cli/internal/cliutil"
)

type gapRow struct {
	SaasCustomerID   string   `json:"saas_customer_id"`
	SaasCustomerName string   `json:"saas_customer_name"`
	OrganizationName string   `json:"organization_name,omitempty"`
	Domains          []string `json:"domains"`
	ProductTypes     []string `json:"product_types"`
	Name             string   `json:"name"`
	MainID           string   `json:"main_id"`
	SeatType         string   `json:"seat_type"`
	SeatState        string   `json:"seat_state"`
	DateAdded        string   `json:"date_added"`
	RemoteID         string   `json:"remote_id"`
	Notes            []string `json:"notes"`
	added            time.Time
	addedOK          bool
}

type gapSubtotal struct {
	SaasCustomerID   string `json:"saas_customer_id"`
	SaasCustomerName string `json:"saas_customer_name"`
	Unprotected      int    `json:"unprotected"`
	Paused           int    `json:"paused"`
	Total            int    `json:"total"`
}

func newNovelSeatsGapsCmd(flags *rootFlags) *cobra.Command {
	var dbPath, customer, since string
	var includePaused, byCustomer bool
	var limit int
	cmd := &cobra.Command{
		Use:   "gaps",
		Short: "List seats across all clients that are Unprotected (Paused too with --include-paused), newest first, with each seat's remoteId for a License plan.",
		Long: strings.Trim(`
Every seat across all synced clients that is not protected: Unprotected by
default, plus Paused with --include-paused. --since keeps seats whose
dateAdded falls inside the window. dateAdded has no documented format, so a
seat whose date cannot be parsed is kept and marked date_unparsed rather than
hidden. --by-customer prints per-client subtotals instead of seat rows.

Use this command to find seats across every client that are not protected (Unprotected by default, Paused with --include-paused), optionally only those added within --since.
Do NOT use this command to check whether protected services are actually backing up; use 'backup-health' instead.
Do NOT use it to prepare License/Pause/Unlicense batches; use 'seats bulk-plan' instead.`, "\n"),
		Example: strings.Trim(`
  datto-saas-cli seats gaps --since 30d --agent
  datto-saas-cli seats gaps --include-paused --by-customer`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "seats gaps")
			}
			var cutoff time.Time
			if since != "" {
				d, err := cliutil.ParseDurationLoose(since)
				if err != nil || d <= 0 {
					return usageErr(fmt.Errorf("--since %q: use a duration such as 30d, 2w or 72h", since))
				}
				cutoff = time.Now().UTC().Add(-d)
			}
			rows := make([]gapRow, 0)
			db, missing, err := openSaasStore(cmd, dbPath)
			if err != nil {
				return err
			}
			if missing {
				if byCustomer {
					return printSaasRows(cmd, flags, make([]gapSubtotal, 0), "No local data. Run: datto-saas-cli sync")
				}
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
				if s.SeatState != "Unprotected" && !(includePaused && s.SeatState == "Paused") {
					continue
				}
				r := gapRow{SaasCustomerID: s.CustomerID, Name: s.Name, MainID: s.MainID, SeatType: s.SeatType,
					SeatState: s.SeatState, DateAdded: string(s.DateAdded), RemoteID: s.RemoteID, Notes: make([]string, 0)}
				c := byID[s.CustomerID]
				r.SaasCustomerName, r.OrganizationName = c.Name, c.OrgName
				r.Domains, r.ProductTypes = c.domainNames(), c.productTypes()
				r.added, r.addedOK = parseSeatDate(string(s.DateAdded))
				if !r.addedOK {
					r.Notes = append(r.Notes, "date_unparsed")
				}
				if strings.TrimSpace(s.RemoteID) == "" {
					r.Notes = append(r.Notes, "no_remote_id")
				}
				if !cutoff.IsZero() && r.addedOK && r.added.Before(cutoff) {
					continue
				}
				rows = append(rows, r)
			}
			// Newest first; unparseable dates sort last so they stay visible.
			sort.SliceStable(rows, func(a, b int) bool {
				if rows[a].addedOK != rows[b].addedOK {
					return rows[a].addedOK
				}
				return rows[a].added.After(rows[b].added)
			})
			if byCustomer {
				subs := make([]gapSubtotal, 0)
				idx := map[string]int{}
				for _, r := range rows {
					i, ok := idx[r.SaasCustomerID]
					if !ok {
						idx[r.SaasCustomerID] = len(subs)
						subs = append(subs, gapSubtotal{SaasCustomerID: r.SaasCustomerID, SaasCustomerName: r.SaasCustomerName})
						i = len(subs) - 1
					}
					subs[i].Total++
					if r.SeatState == "Paused" {
						subs[i].Paused++
					} else {
						subs[i].Unprotected++
					}
				}
				sort.SliceStable(subs, func(a, b int) bool { return subs[a].Total > subs[b].Total })
				return printSaasRows(cmd, flags, subs, emptyMsg(domains, "No unprotected seats."))
			}
			if limit > 0 && len(rows) > limit {
				fmt.Fprintf(cmd.ErrOrStderr(), "showing %d of %d gap seats; raise --limit to see more\n", limit, len(rows))
				rows = rows[:limit]
			}
			return printSaasRows(cmd, flags, rows, emptyMsg(domains, "No unprotected seats."))
		},
	}
	cmd.Flags().StringVar(&since, "since", "", "Only seats added within this window (e.g. 30d, 2w, 72h)")
	cmd.Flags().BoolVar(&includePaused, "include-paused", false, "Also include Paused seats")
	cmd.Flags().BoolVar(&byCustomer, "by-customer", false, "Print per-client subtotals instead of seat rows")
	cmd.Flags().StringVar(&customer, "customer", "", "Limit to one client: saasCustomerId, customer name, or domain")
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum seat rows to print (0 = all)")
	cmd.Flags().StringVar(&dbPath, "db", "", "SQLite database path (default: the CLI's synced store)")
	return cmd
}
