// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source local

package cli

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"datto-saas-pp-cli/internal/cliutil"
)

// maxBulkSeatIDs is Datto's recommended ceiling per bulkSeatChange call (S1).
const maxBulkSeatIDs = 100

var bulkSeatActions = []string{"License", "Pause", "Unlicense"}

// seatTypesByProduct lists the seat types each product accepts (S1).
var seatTypesByProduct = map[string][]string{
	"Office365":  {"User", "SharedMailbox", "Site", "TeamSite", "Team"},
	"GoogleApps": {"User", "SharedDrive"},
}

// alreadyInState names the seat state an action would leave unchanged.
var alreadyInState = map[string]string{"License": "Active", "Pause": "Paused", "Unlicense": "Unprotected"}

type planSeat struct {
	SaasCustomerID   string `json:"saas_customer_id"`
	SaasCustomerName string `json:"saas_customer_name"`
	Name             string `json:"name"`
	MainID           string `json:"main_id"`
	SeatType         string `json:"seat_type"`
	SeatState        string `json:"seat_state"`
	RemoteID         string `json:"remote_id,omitempty"`
	Reason           string `json:"reason"`
}

type planBatch struct {
	SaasCustomerID   string   `json:"saas_customer_id"`
	SaasCustomerName string   `json:"saas_customer_name"`
	Subscription     string   `json:"external_subscription_id"`
	ProductType      string   `json:"product_type"`
	SeatType         string   `json:"seat_type"`
	ActionType       string   `json:"action_type"`
	Count            int      `json:"count"`
	IDs              []string `json:"ids"`
	Seats            []string `json:"seats"`
	Command          string   `json:"command"`
	Warnings         []string `json:"warnings"`
}

type bulkPlan struct {
	Action        string      `json:"action"`
	SeatsSelected int         `json:"seats_selected"`
	SeatsPlanned  int         `json:"seats_planned"`
	Batches       []planBatch `json:"batches"`
	Skipped       []planSeat  `json:"skipped"`
	Blocked       []planSeat  `json:"blocked"`
	Note          string      `json:"note"`
	// MirrorIncomplete lists resources whose last sync did not finish; no
	// batches are planned from an incomplete mirror.
	MirrorIncomplete []string `json:"mirror_incomplete,omitempty"`
}

var shellSafe = regexp.MustCompile(`^[A-Za-z0-9._:@/+=,-]+$`)

func shellQuote(s string) string {
	if shellSafe.MatchString(s) {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func newNovelSeatsBulkPlanCmd(flags *rootFlags) *cobra.Command {
	var dbPath, action, customer, states, seatTypes, since, subscription string
	cmd := &cobra.Command{
		Use:   "bulk-plan",
		Short: "Turn a seat selection into ready-to-run License, Pause or Unlicense batches",
		Long: strings.Trim(`
Selects seats from the local store and plans bulkSeatChange batches without
sending anything. Batches are grouped by client, external subscription and
seat type (one seat type per call) and capped at 100 remote IDs, Datto's
recommended maximum. Each batch carries the exact 'seats bulk-change'
command to run.

A selector is required (--customer, --state, --seat-type or --since) so a
fleet-wide plan is never built by accident. Seats are skipped, with a reason,
when they have no remoteId, their seat type is not valid for the
subscription's product, or they are already in the target state. A client
with more than one external subscription is blocked until --subscription
picks one. Google Workspace batches carry a Seat Management 2.0 warning.

Use this command to turn a seat selection from the local store into ready-to-run bulkSeatChange batches (grouped by customer, subscription and seat type, at most 100 remoteIds each); it never sends a change.
Do NOT use this command just to view unprotected seats; use 'seats gaps' instead.
To execute a batch, run 'seats bulk-change' with that batch's values.`, "\n"),
		Example: strings.Trim(`
  datto-saas-cli seats bulk-plan --action License --state Unprotected --customer 123456 --agent
  datto-saas-cli seats bulk-plan --action Unlicense --state Archived --customer contoso.com`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "seats bulk-plan")
			}
			customer, subscription, since = strings.TrimSpace(customer), strings.TrimSpace(subscription), strings.TrimSpace(since)
			if action == "" {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--action is required: License, Pause or Unlicense"))
			}
			if err := validateEnumSet("action", map[string]bool{action: true}, bulkSeatActions); err != nil {
				return err
			}
			stateSet, typeSet := splitCSVSet(states), splitCSVSet(seatTypes)
			if err := validateEnumSet("state", stateSet, saasSeatStates); err != nil {
				return err
			}
			if err := validateEnumSet("seat-type", typeSet, saasSeatTypes); err != nil {
				return err
			}
			if customer == "" && len(stateSet) == 0 && len(typeSet) == 0 && since == "" {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("refusing a fleet-wide plan: pass at least one selector (--customer, --state, --seat-type or --since)"))
			}
			var cutoff time.Time
			if since != "" {
				d, err := cliutil.ParseDurationLoose(since)
				if err != nil || d <= 0 {
					return usageErr(fmt.Errorf("--since %q: use a duration such as 30d, 2w or 72h", since))
				}
				cutoff = time.Now().UTC().Add(-d)
			}
			plan := bulkPlan{Action: action, Batches: make([]planBatch, 0), Skipped: make([]planSeat, 0), Blocked: make([]planSeat, 0)}
			db, missing, err := openSaasStore(cmd, dbPath)
			if err != nil {
				return err
			}
			if missing {
				plan.Note = "no local data; run datto-saas-cli sync"
				return printJSONFiltered(cmd.OutOrStdout(), plan, flags)
			}
			defer db.Close()
			rd, err := beginSaasRead(cmd, db, flags, "domains", "seats")
			if err != nil {
				return err
			}
			defer rd.Close()
			if partial := rd.partial; len(partial) > 0 {
				// An unfinished domains sync can hide a subscription, and an
				// unfinished seats sync can hide seats: plan nothing.
				plan.MirrorIncomplete = partial
				plan.Note = "no batches planned: the last sync of " + strings.Join(partial, ", ") + " did not finish; run datto-saas-cli sync"
				return printJSONFiltered(cmd.OutOrStdout(), plan, flags)
			}
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
			type groupKey struct{ customer, sub, seatType string }
			groups := map[groupKey][]saasSeat{}
			var order []groupKey
			for _, s := range seats {
				if len(stateSet) > 0 && !stateSet[s.SeatState] {
					continue
				}
				if len(typeSet) > 0 && !typeSet[s.SeatType] {
					continue
				}
				if !cutoff.IsZero() {
					t, ok := parseSeatDate(string(s.DateAdded))
					if !ok || t.Before(cutoff) { // an unknown date never joins a time-scoped change
						continue
					}
				}
				plan.SeatsSelected++
				c := byID[s.CustomerID]
				ps := planSeat{SaasCustomerID: s.CustomerID, SaasCustomerName: c.Name, Name: s.Name, MainID: s.MainID, SeatType: s.SeatType, SeatState: s.SeatState, RemoteID: s.RemoteID}
				subs := c.subscriptions()
				sub := subscription
				switch {
				case len(c.Domains) == 0:
					ps.Reason = "customer_not_in_domains"
				case sub != "" && !contains(subs, sub):
					ps.Reason = "subscription_not_found_for_customer"
				case sub == "" && len(subs) == 0:
					ps.Reason = "missing_subscription"
				case sub == "" && len(subs) > 1:
					ps.Reason = "ambiguous_subscription: pass --subscription (" + strings.Join(subs, ", ") + ")"
				case sub == "" && c.hasDomainWithoutSubscription():
					// One known id plus an unmapped domain is not "exactly one".
					ps.Reason = "incomplete_subscription_mapping: a domain has no externalSubscriptionId; pass --subscription (" + strings.Join(subs, ", ") + ")"
				}
				if ps.Reason != "" {
					plan.Blocked = append(plan.Blocked, ps)
					continue
				}
				if sub == "" {
					sub = subs[0]
				}
				product := ""
				for _, d := range c.Domains {
					if d.SubscriptionID == sub {
						product = d.ProductType
						break
					}
				}
				switch {
				case strings.TrimSpace(s.RemoteID) == "":
					ps.Reason = "no_remote_id"
				case seatTypesByProduct[product] != nil && !contains(seatTypesByProduct[product], s.SeatType):
					ps.Reason = "seat_type_not_allowed_for_product"
				case s.SeatState == alreadyInState[action]:
					ps.Reason = "already_in_target_state"
				}
				if ps.Reason != "" {
					plan.Skipped = append(plan.Skipped, ps)
					continue
				}
				k := groupKey{s.CustomerID, sub, s.SeatType}
				if _, ok := groups[k]; !ok {
					order = append(order, k)
				}
				groups[k] = append(groups[k], s)
			}
			sort.SliceStable(order, func(a, b int) bool {
				if order[a].customer != order[b].customer {
					return order[a].customer < order[b].customer
				}
				if order[a].sub != order[b].sub {
					return order[a].sub < order[b].sub
				}
				return order[a].seatType < order[b].seatType
			})
			for _, k := range order {
				c := byID[k.customer]
				product := ""
				for _, d := range c.Domains {
					if d.SubscriptionID == k.sub {
						product = d.ProductType
					}
				}
				members := groups[k]
				for start := 0; start < len(members); start += maxBulkSeatIDs {
					end := start + maxBulkSeatIDs
					if end > len(members) {
						end = len(members)
					}
					b := planBatch{SaasCustomerID: k.customer, SaasCustomerName: c.Name, Subscription: k.sub, ProductType: product,
						SeatType: k.seatType, ActionType: action, Warnings: make([]string, 0)}
					for _, s := range members[start:end] {
						b.IDs = append(b.IDs, s.RemoteID)
						b.Seats = append(b.Seats, s.MainID)
					}
					b.Count = len(b.IDs)
					// --ids is a JSON array so an id containing a comma survives the
					// generated parser (cliutil.ParseStringList) byte for byte.
					idsJSON, err := json.Marshal(b.IDs)
					if err != nil {
						return err
					}
					b.Command = fmt.Sprintf("datto-saas-cli seats bulk-change %s %s --seat-type %s --action-type %s --ids %s",
						shellQuote(k.customer), shellQuote(k.sub), shellQuote(k.seatType), shellQuote(action), shellQuote(string(idsJSON)))
					if product == "GoogleApps" {
						b.Warnings = append(b.Warnings, "requires_seat_management_2")
					}
					if len(c.subscriptions()) > 1 || c.hasDomainWithoutSubscription() {
						// Seats are listed per customer, not per subscription, so the
						// pairing below is the operator's --subscription choice.
						b.Warnings = append(b.Warnings, "customer_subscription_mapping_not_unique: confirm these seats belong to "+k.sub)
					}
					if seatTypesByProduct[product] == nil {
						b.Warnings = append(b.Warnings, "unknown_product_type: seat type not checked against product")
					}
					plan.SeatsPlanned += b.Count
					plan.Batches = append(plan.Batches, b)
				}
			}
			switch {
			case len(domains) == 0:
				plan.Note = "no synced data; run datto-saas-cli sync"
			case plan.SeatsSelected == 0:
				plan.Note = "no seats matched the selector"
			case len(plan.Batches) == 0:
				plan.Note = "no seats to change; see skipped and blocked for reasons"
			default:
				plan.Note = "nothing was sent; run each batch command to apply it"
			}
			return printJSONFiltered(cmd.OutOrStdout(), plan, flags)
		},
	}
	cmd.Flags().StringVar(&action, "action", "", "Action to plan (case-sensitive): License, Pause or Unlicense")
	cmd.Flags().StringVar(&customer, "customer", "", "Limit to one client: saasCustomerId, customer name, or domain")
	cmd.Flags().StringVar(&states, "state", "", "Comma-separated seat states to select: Active, Paused, Archived, Unprotected")
	cmd.Flags().StringVar(&seatTypes, "seat-type", "", "Comma-separated seat types to select: User, SharedMailbox, SharedDrive, Site, TeamSite, Team")
	cmd.Flags().StringVar(&since, "since", "", "Only seats added within this window (e.g. 30d); seats with unparseable dates are excluded")
	cmd.Flags().StringVar(&subscription, "subscription", "", "External subscription id to use when a client has several (e.g. Classic:Office365:654321)")
	cmd.Flags().StringVar(&dbPath, "db", "", "SQLite database path (default: the CLI's synced store)")
	return cmd
}
