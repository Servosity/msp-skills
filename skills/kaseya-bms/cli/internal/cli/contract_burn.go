// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel command: contract-burn.

// pp:data-source local

package cli

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

type contractBurnItem struct {
	Contract             string   `json:"contract"`
	ContractID           string   `json:"contract_id,omitempty"`
	Account              string   `json:"account"`
	Type                 string   `json:"type,omitempty"`
	Status               string   `json:"status,omitempty"`
	BillingCycle         string   `json:"billing_cycle,omitempty"`
	StartDate            string   `json:"start_date,omitempty"`
	EndDate              string   `json:"end_date,omitempty"`
	DaysRemaining        *int     `json:"days_remaining,omitempty"`
	PercentPeriodElapsed *float64 `json:"percent_period_elapsed,omitempty"`
	HoursConsumed        float64  `json:"hours_consumed"`
	OpenTickets          int      `json:"open_tickets"`
}

type contractBurnView struct {
	Items      []contractBurnItem `json:"items"`
	WindowDays int                `json:"window_days"`
	Note       string             `json:"note,omitempty"`
}

// aggregateContractBurn joins contracts with hours consumed (time logs routed
// through their tickets' contracts) and open-ticket counts. The BMS API does
// not expose allotted block hours, so depletion is expressed as consumed
// hours plus contract-period elapsed - honest signals, not invented quotas.
// Pure function for table-driven tests.
func aggregateContractBurn(contractRows, ticketRows, timelogRows []map[string]any, windowDays int, unit string, now time.Time) contractBurnView {
	view := contractBurnView{WindowDays: windowDays, Items: []contractBurnItem{}}

	// Contract metadata from the synced finance-contracts-summary mirror,
	// keyed by contract identity (kaseya-bms handfix record-identity-by-id;
	// Codex review r1 class contract identity collision: two "Support"
	// contracts of different accounts were one item, their tickets and hours
	// combined): ClientContractSummaryOutputDto.Id joins
	// TicketListDto.ContractId; a record without an id keys on its contract
	// number, then on its name and account.
	type meta struct {
		id, name, account, ctype, status, cycle string
		start, end                              time.Time
		hasStart, hasEnd                        bool
	}
	contracts := map[string]*meta{}
	for _, m := range contractRows {
		key := kbmsContractKey(m, "Id")
		if key == "" {
			continue
		}
		name := kbmsStr(m, "ContractName")
		if name == "" {
			name = kbmsStr(m, "Name")
		}
		cm := &meta{
			id:      kbmsIDString(m, "Id"),
			name:    name,
			account: kbmsStr(m, "AccountName"),
			ctype:   kbmsStr(m, "ContractType"),
			status:  kbmsStr(m, "Status"),
			cycle:   kbmsStr(m, "BillingCycle"),
		}
		cm.start, cm.hasStart = kbmsTime(m, "StartDate")
		cm.end, cm.hasEnd = kbmsTime(m, "EndDate")
		contracts[key] = cm
	}

	// Ticket -> contract routing plus open-ticket counts per contract.
	ticketContract := map[string]string{}
	openByContract := map[string]int{}
	accountByContract := map[string]string{}
	nameByContract := map[string]string{}
	idByContract := map[string]string{}
	for _, m := range ticketRows {
		key := kbmsContractKey(m, "ContractId")
		if key == "" {
			continue
		}
		if id := kbmsIDString(m, "Id"); id != "" {
			ticketContract[id] = key
		}
		if accountByContract[key] == "" {
			accountByContract[key] = kbmsStr(m, "AccountName")
		}
		if nameByContract[key] == "" {
			nameByContract[key] = kbmsStr(m, "ContractName")
		}
		if idByContract[key] == "" {
			idByContract[key] = kbmsIDString(m, "ContractId")
		}
		if kbmsTicketOpen(m) {
			openByContract[key]++
		}
	}

	// Hours consumed per contract inside the window, via ticket routing.
	cutoff := now.AddDate(0, 0, -windowDays)
	hoursByContract := map[string]float64{}
	for _, m := range timelogRows {
		started, ok := kbmsTime(m, "StartDate")
		if !ok {
			started, ok = kbmsTime(m, "CreatedOn")
		}
		if !ok || started.Before(cutoff) {
			continue
		}
		key := ticketContract[kbmsIDString(m, "TicketId")]
		if key == "" {
			continue
		}
		if raw, ok := kbmsNum(m, "Timespent"); ok {
			hoursByContract[key] += kbmsHoursFromTimespent(raw, unit)
		}
	}

	// Union of contracts seen anywhere.
	keys := map[string]bool{}
	for key := range contracts {
		keys[key] = true
	}
	for key := range openByContract {
		keys[key] = true
	}
	for key := range hoursByContract {
		keys[key] = true
	}

	for _, key := range kbmsSortedKeys(keys) {
		item := contractBurnItem{
			Contract:      nameByContract[key],
			ContractID:    idByContract[key],
			HoursConsumed: kbmsRound2(hoursByContract[key]),
			OpenTickets:   openByContract[key],
		}
		if cm, ok := contracts[key]; ok {
			if cm.name != "" {
				item.Contract = cm.name
			}
			if cm.id != "" {
				item.ContractID = cm.id
			}
			item.Account = cm.account
			item.Type = cm.ctype
			item.Status = cm.status
			item.BillingCycle = cm.cycle
			if cm.hasStart {
				item.StartDate = cm.start.Format("2006-01-02")
			}
			if cm.hasEnd {
				item.EndDate = cm.end.Format("2006-01-02")
				days := int(cm.end.Sub(now).Hours() / 24)
				item.DaysRemaining = &days
			}
			if cm.hasStart && cm.hasEnd && cm.end.After(cm.start) {
				pct := kbmsRound2(100 * now.Sub(cm.start).Hours() / cm.end.Sub(cm.start).Hours())
				if pct < 0 {
					pct = 0
				}
				if pct > 100 {
					pct = 100
				}
				item.PercentPeriodElapsed = &pct
			}
		}
		if item.Account == "" {
			item.Account = accountByContract[key]
		}
		view.Items = append(view.Items, item)
	}
	sort.Slice(view.Items, func(i, j int) bool {
		if view.Items[i].HoursConsumed != view.Items[j].HoursConsumed {
			return view.Items[i].HoursConsumed > view.Items[j].HoursConsumed
		}
		if view.Items[i].Contract != view.Items[j].Contract {
			return view.Items[i].Contract < view.Items[j].Contract
		}
		return view.Items[i].ContractID < view.Items[j].ContractID
	})
	if len(view.Items) == 0 {
		view.Note = "no contracts in the local mirror; run 'sync --resources finance-contracts-summary,servicedesk,timelogs' to refresh"
	}
	return view
}

func newNovelContractBurnCmd(flags *rootFlags) *cobra.Command {
	var windowDays int
	var unit string
	var dbPath string

	cmd := &cobra.Command{
		Use:   "contract-burn",
		Short: "Per-contract burn picture: hours consumed, open tickets, and how much of the contract period has elapsed - at-risk agreements surface first.",
		Long: strings.Trim(`
Use this command for per-contract consumption and period-elapsed depletion.
Do NOT use this command for amounts ready to invoice; use 'unbilled' instead.

The BMS API does not expose allotted block hours, so this reports the honest
signals it can compute: hours consumed in the window (time logs routed through
their tickets' contracts), open tickets per contract, and percent of the
contract period elapsed from the synced contract summary.`, "\n"),
		Example: strings.Trim(`
  # Fleet-wide burn, busiest contracts first
  kaseya-bms-cli contract-burn --agent

  # Quarter-to-date consumption with agent-trimmed fields
  kaseya-bms-cli contract-burn --window-days 90 --agent --select items.account,items.contract,items.hours_consumed`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would join contracts, tickets, and time logs into a per-contract burn view")
				return nil
			}
			if err := kbmsRejectLiveSource(flags, "contract-burn"); err != nil {
				return err
			}
			if dbPath == "" {
				dbPath = defaultDBPath("kaseya-bms-cli")
			}
			db, err := kbmsOpenStore(cmd.Context(), dbPath)
			if err != nil {
				return fmt.Errorf("opening database: %w", err)
			}
			defer db.Close()
			if !hintIfUnsynced(cmd, db, "finance-contracts-summary") {
				hintIfStale(cmd, db, "finance-contracts-summary", flags.maxAge)
			}
			contractRows, err := kbmsRows(cmd.Context(), db, "finance-contracts-summary")
			if err != nil {
				return fmt.Errorf("querying contracts: %w", err)
			}
			ticketRows, err := kbmsRows(cmd.Context(), db, "servicedesk")
			if err != nil {
				return fmt.Errorf("querying tickets: %w", err)
			}
			timelogRows, err := kbmsRows(cmd.Context(), db, "timelogs")
			if err != nil {
				return fmt.Errorf("querying time logs: %w", err)
			}
			view := aggregateContractBurn(contractRows, ticketRows, timelogRows, windowDays, unit, time.Now().UTC())
			return printJSONFiltered(cmd.OutOrStdout(), view, flags)
		},
	}
	cmd.Flags().IntVar(&windowDays, "window-days", 90, "Window for the hours-consumed column")
	cmd.Flags().StringVar(&unit, "timespent-unit", "minutes", "Unit BMS uses for Timespent values: minutes, hours, or seconds")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default ~/.local/share/kaseya-bms-cli/data.db)")
	return cmd
}

// kbmsContractKey is a contract's identity on a contract or ticket row: its id
// (idField: Id on a contract, ContractId on a ticket), else its contract
// number, else its name and account; "" when the row names no contract.
func kbmsContractKey(m map[string]any, idField string) string {
	if id := kbmsIDString(m, idField); id != "" && id != "0" {
		return "id:" + id
	}
	if n := strings.TrimSpace(kbmsStr(m, "ContractNumber")); n != "" {
		return "number:" + strings.ToLower(n)
	}
	name := kbmsStr(m, "ContractName")
	if name == "" {
		name = kbmsStr(m, "Name")
	}
	if name == "" {
		return ""
	}
	return "name:" + kbmsNormName(name) + "|" + kbmsNormName(kbmsStr(m, "AccountName"))
}
