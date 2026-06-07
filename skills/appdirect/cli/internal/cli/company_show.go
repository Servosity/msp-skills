// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-implemented novel feature: company 360 view joining the local
// companies, users, subscriptions, invoices, and opportunities tables.
// Originally scaffolded by the CLI Printing Press; body is hand-authored.

package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"appdirect-pp-cli/internal/store"
)

// pp:data-source local

type company360User struct {
	ID        string `json:"id"`
	Email     string `json:"email,omitempty"`
	FirstName string `json:"firstName,omitempty"`
	LastName  string `json:"lastName,omitempty"`
}

type company360Sub struct {
	SubscriptionID string `json:"subscriptionId"`
	Product        string `json:"product,omitempty"`
	Status         string `json:"status,omitempty"`
	CreatedOn      string `json:"createdOn,omitempty"`
	EndDate        string `json:"endDate,omitempty"`
}

type company360Invoice struct {
	InvoiceID string  `json:"invoiceId"`
	Status    string  `json:"status,omitempty"`
	Total     float64 `json:"total,omitempty"`
	Currency  string  `json:"currency,omitempty"`
	DueDate   string  `json:"dueDate,omitempty"`
}

type company360Opp struct {
	ID         string `json:"id"`
	Name       string `json:"name,omitempty"`
	Status     string `json:"status,omitempty"`
	OwnerEmail string `json:"ownerEmail,omitempty"`
	CreatedOn  string `json:"createdOn,omitempty"`
}

type company360View struct {
	Company       map[string]any      `json:"company"`
	Users         []company360User    `json:"users"`
	Subscriptions []company360Sub     `json:"subscriptions"`
	Invoices      []company360Invoice `json:"invoices"`
	Opportunities []company360Opp     `json:"opportunities"`
	Counts        map[string]int      `json:"counts"`
	Note          string              `json:"note,omitempty"`
}

func newNovelCompanyShowCmd(flags *rootFlags) *cobra.Command {
	var dbPath string

	cmd := &cobra.Command{
		Use:   "show [companyId]",
		Short: "One customer's full picture - users, subscriptions, invoices, and open opportunities",
		Long: strings.TrimSpace(`
Use this command for one customer's full picture: the company record plus its
users, subscriptions, invoices, and open opportunities, joined from the local
store in a single payload. Pass the company UUID (as shown by
'search <name> --type account-v2-companies' or the companies endpoints).

Do NOT use this command for marketplace-wide billing matching; use 'reconcile'
instead. Run 'sync' first to populate the local store.`),
		Example: strings.Trim(`
  # Full snapshot for a support ticket
  appdirect-cli company show 12345678-aaaa-bbbb-cccc-1234567890ab --json

  # Just the billing slice
  appdirect-cli company show 12345678-aaaa-bbbb-cccc-1234567890ab --agent --select company.name,invoices,counts
`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only": "true",
			"pp:happy-args": "companyId=12345678-aaaa-bbbb-cccc-1234567890ab",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would join local company, users, subscriptions, invoices, and opportunities")
				return nil
			}
			if len(args) < 1 || strings.TrimSpace(args[0]) == "" {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("companyId argument is required"))
			}
			companyID := strings.TrimSpace(args[0])

			if dbPath == "" {
				dbPath = defaultDBPath("appdirect-cli")
			}
			db, err := store.OpenWithContext(cmd.Context(), dbPath)
			if err != nil {
				return fmt.Errorf("opening database: %w", err)
			}
			defer db.Close()

			for _, rt := range []string{rtCompanies, rtUsers, rtSubscriptions, rtInvoices, rtOpportunities} {
				if !hintIfUnsynced(cmd, db, rt) {
					hintIfStale(cmd, db, rt, flags.maxAge)
				}
			}

			companies, err := loadResourceObjects(cmd.Context(), db, rtCompanies)
			if err != nil {
				return err
			}
			var company map[string]any
			for _, c := range companies {
				if novelStr(c, "uuid") == companyID || novelStr(c, "id") == companyID {
					company = c
					break
				}
			}

			users, err := loadResourceObjects(cmd.Context(), db, rtUsers)
			if err != nil {
				return err
			}
			matchedUsers := make([]company360User, 0)
			for _, u := range users {
				if novelRefID(u, "company") != companyID {
					continue
				}
				matchedUsers = append(matchedUsers, company360User{
					ID:        novelStr(u, "id"),
					Email:     novelStr(u, "email"),
					FirstName: novelStr(u, "firstName"),
					LastName:  novelStr(u, "lastName"),
				})
			}

			subs, err := loadResourceObjects(cmd.Context(), db, rtSubscriptions)
			if err != nil {
				return err
			}
			matchedSubs := make([]company360Sub, 0)
			for _, s := range subs {
				if novelRefID(s, "company") != companyID {
					continue
				}
				created, _ := novelEpochMS(s, "creationDate")
				end, _ := novelEpochMS(s, "endDate")
				matchedSubs = append(matchedSubs, company360Sub{
					SubscriptionID: novelStr(s, "id"),
					Product:        novelRefName(s, "product"),
					Status:         strings.ToUpper(novelStr(s, "status")),
					CreatedOn:      isoOrEmpty(created),
					EndDate:        isoOrEmpty(end),
				})
			}

			invoices, err := loadResourceObjects(cmd.Context(), db, rtInvoices)
			if err != nil {
				return err
			}
			matchedInvoices := make([]company360Invoice, 0)
			for _, inv := range invoices {
				if novelRefID(inv, "company") != companyID {
					continue
				}
				total, _ := novelNum(inv, "total")
				due, _ := novelEpochMS(inv, "dueDate")
				matchedInvoices = append(matchedInvoices, company360Invoice{
					InvoiceID: novelStr(inv, "invoiceId"),
					Status:    strings.ToUpper(novelStr(inv, "status")),
					Total:     total,
					Currency:  novelStr(inv, "currency"),
					DueDate:   isoOrEmpty(due),
				})
			}

			opps, err := loadResourceObjects(cmd.Context(), db, rtOpportunities)
			if err != nil {
				return err
			}
			matchedOpps := make([]company360Opp, 0)
			for _, o := range opps {
				customer := novelNested(o, "customerUser")
				if customer == nil || novelRefID(customer, "company") != companyID {
					continue
				}
				if !strings.EqualFold(novelStr(o, "status"), "OPEN") {
					continue
				}
				owner := novelNested(o, "ownerUser")
				created, _ := novelEpochMS(o, "createdOn")
				matchedOpps = append(matchedOpps, company360Opp{
					ID:         novelStr(o, "id"),
					Name:       novelStr(o, "name"),
					Status:     strings.ToUpper(novelStr(o, "status")),
					OwnerEmail: novelStr(owner, "email"),
					CreatedOn:  isoOrEmpty(created),
				})
			}

			view := company360View{
				Company:       company,
				Users:         matchedUsers,
				Subscriptions: matchedSubs,
				Invoices:      matchedInvoices,
				Opportunities: matchedOpps,
				Counts: map[string]int{
					"users":         len(matchedUsers),
					"subscriptions": len(matchedSubs),
					"invoices":      len(matchedInvoices),
					"opportunities": len(matchedOpps),
				},
			}
			if company == nil {
				view.Note = fmt.Sprintf("company %q not found in the local store; run 'sync --resources account-v2-companies' or check the UUID", companyID)
			}
			return printJSONFiltered(cmd.OutOrStdout(), view, flags)
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	return cmd
}
