// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"jira-service-management-pp-cli/internal/cliutil"
)

type rosterCustomer struct {
	AccountID   string   `json:"account_id"`
	DisplayName string   `json:"display_name,omitempty"`
	Email       string   `json:"email,omitempty"`
	Orgs        []string `json:"orgs,omitempty"`
}

type rosterOrg struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Members       int    `json:"members"`
	OnDesk        bool   `json:"on_desk"`
	DeskCustomers int    `json:"desk_customers"`
}

type rosterView struct {
	DeskID                  string           `json:"desk_id"`
	Customers               int              `json:"customers"`
	Organizations           []rosterOrg      `json:"organizations"`
	CustomersWithoutOrg     []rosterCustomer `json:"customers_without_org"`
	CustomersOrgUnknown     []rosterCustomer `json:"customers_org_unknown,omitempty"`
	CustomersInMultipleOrgs []rosterCustomer `json:"customers_in_multiple_orgs"`
	EmptyOrgs               []rosterOrg      `json:"empty_orgs"`
	OrgsNotOnDesk           []rosterOrg      `json:"orgs_not_on_desk"`
	ScannedOrgs             int              `json:"scanned_orgs"`
	Truncated               bool             `json:"truncated,omitempty"`
	Note                    string           `json:"note,omitempty"`
}

// rosterMaxCustomerPages caps the customer walk at 100,000 customers.
const rosterMaxCustomerPages = 2000

func newNovelRosterCmd(flags *rootFlags) *cobra.Command {
	var maxOrgs int

	cmd := &cobra.Command{
		Use:   "roster <desk-id>",
		Short: "Full customer list for a desk with membership problems flagged: no org, two orgs, empty orgs, orgs not on the desk.",
		Long: strings.Trim(`
Reads every customer on the service desk, paging until an empty page or a short
page JSM does not mark isLastPage:false, and de-duplicating by accountId. The
customers endpoint returns at most 50 people a page, and its paging metadata has
been unreliable (JSDCLOUD-8012, JSDCLOUD-8279).
Then it joins customers to organizations and their members and reports:
customers in no organization, customers in more than one, empty organizations,
and organizations that hold this desk's customers but are not attached to it.

Use this command for customer and organization membership hygiene on a desk.
Do NOT use this command for a client organization's tickets or SLA picture; use 'client' instead.`, "\n"),
		Example: strings.Trim(`
  jira-service-management-cli roster 3 --agent
  jira-service-management-cli roster 3 --select customers_without_org`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live", "pp:happy-args": "desk=3"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "roster")
			}
			if len(args) != 1 || strings.TrimSpace(args[0]) == "" {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("a service desk id is required (see 'servicedesk list')"))
			}
			desk, segErr := jsmPathSeg(args[0])
			if segErr != nil {
				return usageErr(segErr)
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			if maxOrgs < 1 {
				return usageErr(fmt.Errorf("--max-orgs must be at least 1"))
			}
			orgsTruncated := false
			if cliutil.IsDogfoodEnv() && maxOrgs > 5 {
				maxOrgs = 5
			}

			// An explicit page cap reports truncation instead of failing at
			// the walk ceiling on huge open portals.
			custItems, custTruncated, err := jsmWalk(ctx, c, "/servicedesk/"+desk+"/customer", nil, rosterMaxCustomerPages, jsonKey("accountId"))
			if err != nil {
				return classifyAPIError(cmd.OutOrStdout(), err, flags)
			}
			customers := map[string]*rosterCustomer{}
			for _, raw := range custItems {
				var u jsmUser
				if json.Unmarshal(raw, &u) != nil || u.AccountID == "" {
					continue
				}
				customers[u.AccountID] = &rosterCustomer{AccountID: u.AccountID, DisplayName: u.DisplayName, Email: u.EmailAddress}
			}

			deskOrgItems, _, err := jsmWalk(ctx, c, "/servicedesk/"+desk+"/organization", nil, 0, jsonKey("id"))
			if err != nil {
				return classifyAPIError(cmd.OutOrStdout(), err, flags)
			}
			onDesk := map[string]bool{}
			for _, raw := range deskOrgItems {
				var o jsmOrg
				if json.Unmarshal(raw, &o) == nil {
					onDesk[string(o.ID)] = true
				}
			}
			allOrgItems, _, err := jsmWalk(ctx, c, "/organization", nil, 0, jsonKey("id"))
			if err != nil {
				return classifyAPIError(cmd.OutOrStdout(), err, flags)
			}
			orgs := make([]rosterOrg, 0, len(allOrgItems))
			for _, raw := range allOrgItems {
				var o jsmOrg
				if json.Unmarshal(raw, &o) == nil && o.ID != "" {
					orgs = append(orgs, rosterOrg{ID: string(o.ID), Name: o.Name, OnDesk: onDesk[string(o.ID)]})
				}
			}
			// Desk orgs first so a cap never drops the orgs that matter most.
			sort.SliceStable(orgs, func(i, j int) bool { return orgs[i].OnDesk && !orgs[j].OnDesk })

			view := rosterView{DeskID: desk, Customers: len(customers), Organizations: make([]rosterOrg, 0),
				CustomersWithoutOrg: make([]rosterCustomer, 0), CustomersInMultipleOrgs: make([]rosterCustomer, 0),
				EmptyOrgs: make([]rosterOrg, 0), OrgsNotOnDesk: make([]rosterOrg, 0)}
			for i := range orgs {
				if i >= maxOrgs {
					orgsTruncated = true
					break
				}
				o := &orgs[i]
				oseg, err := jsmPathSeg(o.ID)
				if err != nil {
					continue
				}
				members, _, err := jsmWalk(ctx, c, "/organization/"+oseg+"/user", nil, 0, jsonKey("accountId"))
				if err != nil {
					return classifyAPIError(cmd.OutOrStdout(), err, flags)
				}
				view.ScannedOrgs++
				o.Members = len(members)
				for _, raw := range members {
					id := jsonKey("accountId")(raw)
					if cu, ok := customers[id]; ok {
						cu.Orgs = append(cu.Orgs, o.Name)
						o.DeskCustomers++
					}
				}
				view.Organizations = append(view.Organizations, *o)
				if o.Members == 0 {
					view.EmptyOrgs = append(view.EmptyOrgs, *o)
				}
				if !o.OnDesk && o.DeskCustomers > 0 {
					view.OrgsNotOnDesk = append(view.OrgsNotOnDesk, *o)
				}
			}
			ids := make([]string, 0, len(customers))
			for id := range customers {
				ids = append(ids, id)
			}
			sort.Strings(ids)
			for _, id := range ids {
				cu := customers[id]
				switch {
				case len(cu.Orgs) == 0 && orgsTruncated:
					// Not every org was checked, so "no org" is unproven.
					view.CustomersOrgUnknown = append(view.CustomersOrgUnknown, *cu)
				case len(cu.Orgs) == 0:
					view.CustomersWithoutOrg = append(view.CustomersWithoutOrg, *cu)
				case len(cu.Orgs) > 1:
					view.CustomersInMultipleOrgs = append(view.CustomersInMultipleOrgs, *cu)
				}
			}
			if custTruncated {
				view.Truncated = true
				if len(custItems) >= rosterMaxCustomerPages*jsmMaxPageSize {
					view.Note = "customer list stopped at 100,000 customers; membership results cover those only. "
				} else {
					view.Note = fmt.Sprintf("customer list stopped after %d customers because JSM stopped advancing pages; membership results cover those only. ", len(custItems))
				}
			}
			if orgsTruncated {
				view.Truncated = true
				view.Note += fmt.Sprintf("checked members of %d organizations; customers found in none of them are listed under customers_org_unknown; raise --max-orgs to check the rest", view.ScannedOrgs)
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), view, flags)
			}
			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "desk %s: %d customers, %d organizations checked\n", termSafe(desk), view.Customers, view.ScannedOrgs)
			fmt.Fprintf(w, "no organization: %d\nin 2+ organizations: %d\nempty organizations: %d\norgs with desk customers but not on desk: %d\n",
				len(view.CustomersWithoutOrg), len(view.CustomersInMultipleOrgs), len(view.EmptyOrgs), len(view.OrgsNotOnDesk))
			for _, cu := range view.CustomersInMultipleOrgs {
				fmt.Fprintf(w, "  multi-org: %s (%s) -> %s\n", termSafe(cu.DisplayName), termSafe(cu.Email), termSafe(strings.Join(cu.Orgs, ", ")))
			}
			if view.Note != "" {
				fmt.Fprintln(w, termSafe(view.Note))
			}
			return nil
		},
	}
	cmd.Flags().IntVar(&maxOrgs, "max-orgs", 200, "Maximum organizations whose members are read (desk organizations first)")
	return cmd
}
